#!/usr/bin/env node
// Accessibility gate: run axe-core against a built app in a real browser.
//
// Why this shape: @axe-core/cli downloads chromedriver from a Google host that is
// geo-blocked here, and there is no Chrome in this environment — but Lightpanda speaks
// CDP and runs axe-core fine. So the runner drives Lightpanda directly over the
// WebSocket CDP protocol with Node's built-in WebSocket (no dependencies).
//
// Usage:
//   node scripts/a11y-audit.mjs <dist-dir> <path> [<path> ...]
// Example:
//   node scripts/a11y-audit.mjs web/magazine/dist / /#/topics
//
// Exit code is 1 when any page has a violation, so CI can gate on it.

import { spawn } from 'node:child_process'
import { createServer } from 'node:http'
import { readFileSync, existsSync, copyFileSync } from 'node:fs'
import { join, extname } from 'node:path'
import { setTimeout as sleep } from 'node:timers/promises'

const [distDir, ...paths] = process.argv.slice(2)
if (!distDir || paths.length === 0) {
  console.error('usage: a11y-audit.mjs <dist-dir> <path> [<path> ...]')
  process.exit(2)
}
if (!existsSync(distDir)) {
  console.error(`no such directory: ${distDir}`)
  process.exit(2)
}

// axe is served next to the app so the page can load it with a <script> tag: a 650 KB
// Runtime.evaluate payload over CDP breaks the channel.
const axeSrc = 'web/node_modules/axe-core/axe.min.js'
copyFileSync(axeSrc, join(distDir, 'axe.min.js'))

const MIME = {
  '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css',
  '.json': 'application/json', '.svg': 'image/svg+xml', '.webp': 'image/webp',
}

const server = createServer((req, res) => {
  const url = new URL(req.url, 'http://x')
  let file = join(distDir, url.pathname === '/' ? 'index.html' : url.pathname)
  if (!existsSync(file)) file = join(distDir, 'index.html') // SPA fallback
  try {
    const body = readFileSync(file)
    res.writeHead(200, { 'Content-Type': MIME[extname(file)] ?? 'application/octet-stream' })
    res.end(body)
  } catch {
    res.writeHead(404).end('not found')
  }
})
await new Promise((r) => server.listen(0, '127.0.0.1', r))
const base = `http://127.0.0.1:${server.address().port}`

const lightpanda = process.env.LIGHTPANDA ?? `${process.env.HOME}/.local/bin/lightpanda`
const cdpPort = 9333
const browser = spawn(lightpanda, ['serve', '--host', '127.0.0.1', '--port', String(cdpPort)], {
  stdio: 'ignore',
})

async function waitForCdp() {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${cdpPort}/json/version`)
      if (r.ok) return (await r.json()).webSocketDebuggerUrl
    } catch {}
    await sleep(250)
  }
  throw new Error('lightpanda CDP did not come up')
}

const wsUrl = await waitForCdp()
const ws = new WebSocket(wsUrl)
await new Promise((res, rej) => {
  ws.addEventListener('open', res, { once: true })
  ws.addEventListener('error', rej, { once: true })
})

let nextId = 1
const pending = new Map()
ws.addEventListener('message', (ev) => {
  const msg = JSON.parse(ev.data)
  if (msg.id && pending.has(msg.id)) {
    const { resolve, reject } = pending.get(msg.id)
    pending.delete(msg.id)
    msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result)
  }
})

function send(method, params = {}, sessionId) {
  const id = nextId++
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject })
    ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }))
  })
}

let failed = 0
for (const path of paths) {
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' })
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true })
  await send('Page.enable', {}, sessionId)
  await send('Runtime.enable', {}, sessionId)
  await send('Page.navigate', { url: base + path }, sessionId)
  await sleep(2500) // let the SPA render and fetch

  await send('Runtime.evaluate', {
    expression: `new Promise((res, rej) => { const s = document.createElement('script');
      s.src = '/axe.min.js'; s.onload = res; s.onerror = () => rej(new Error('axe load'));
      document.head.appendChild(s); })`,
    awaitPromise: true,
  }, sessionId)

  const res = await send('Runtime.evaluate', {
    expression: `(async () => {
      const r = await window.axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa'] } });
      return JSON.stringify({ violations: r.violations.map(v => ({
        id: v.id, impact: v.impact, help: v.help,
        nodes: v.nodes.slice(0, 3).map(n => n.target.join(' ')),
      })), passes: r.passes.length });
    })()`,
    awaitPromise: true,
  }, sessionId)

  const out = JSON.parse(res.result.value)
  const tag = `${path} (${out.passes} rules passed)`
  if (out.violations.length === 0) {
    console.log(`PASS ${tag}`)
  } else {
    failed++
    console.error(`FAIL ${tag}`)
    for (const v of out.violations) {
      console.error(`  ${v.impact ?? 'n/a'} ${v.id}: ${v.help} → ${v.nodes.join(' | ')}`)
    }
  }
  await send('Target.closeTarget', { targetId })
}

ws.close()
browser.kill()
server.close()
console.log(failed === 0 ? 'a11y: clean' : `a11y: ${failed} page(s) with violations`)
process.exit(failed === 0 ? 0 : 1)
