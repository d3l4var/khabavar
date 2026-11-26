import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
// KHABAVAR — shared UI primitives (P2 design system).
//
// Small, dependency-free components used by all four apps. They consume design tokens
// only (no hard-coded colours) and use logical CSS properties so RTL is native.
import { useEffect, useState } from 'react';
export function Container({ children, className }) {
    return (_jsx("div", { className: className, style: { maxWidth: 'var(--kh-container)', margin: '0 auto', padding: '0 var(--kh-space-4)' }, children: children }));
}
export function Button({ children, onClick, href, variant = 'primary', type = 'button', disabled, }) {
    const style = {
        display: 'inline-flex',
        alignItems: 'center',
        gap: 'var(--kh-space-2)',
        padding: 'var(--kh-space-3) var(--kh-space-5)',
        borderRadius: 'var(--kh-radius-sm)',
        border: variant === 'primary' ? '1px solid transparent' : '1px solid var(--kh-border)',
        background: variant === 'primary' ? 'var(--kh-accent-strong)' : 'transparent',
        color: variant === 'primary' ? 'var(--kh-on-accent)' : 'var(--kh-text)',
        font: 'inherit',
        cursor: disabled ? 'not-allowed' : 'pointer',
        opacity: disabled ? 0.6 : 1,
    };
    if (href) {
        return (_jsx("a", { href: href, style: style, children: children }));
    }
    return (_jsx("button", { type: type, onClick: onClick, disabled: disabled, style: style, children: children }));
}
export function Badge({ children, tone = 'neutral' }) {
    const color = tone === 'success' ? 'var(--kh-success)' : tone === 'warning' ? 'var(--kh-warning)' : tone === 'error' ? 'var(--kh-error)' : 'var(--kh-accent)';
    return (_jsx("span", { style: {
            display: 'inline-block',
            padding: '2px var(--kh-space-2)',
            borderRadius: '999px',
            border: `1px solid ${color}`,
            color,
            fontSize: 'var(--kh-text-sm)',
        }, children: children }));
}
export function Card({ children }) {
    return (_jsx("article", { style: {
            background: 'var(--kh-surface)',
            border: '1px solid var(--kh-border)',
            borderRadius: 'var(--kh-radius)',
            padding: 'var(--kh-space-4)',
            boxShadow: 'var(--kh-shadow)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--kh-space-2)',
        }, children: children }));
}
// StateBlock renders the three shared states the design system requires
// (Loading / Empty / Error) so every screen behaves the same way.
export function StateBlock({ kind, message, onRetry }) {
    const text = kind === 'loading' ? 'در حال بارگذاری…' : kind === 'empty' ? (message ?? 'موردی برای نمایش نیست.') : (message ?? 'خطایی رخ داد.');
    const color = kind === 'error' ? 'var(--kh-error)' : 'var(--kh-text-muted)';
    return (_jsxs("div", { role: kind === 'error' ? 'alert' : 'status', style: { padding: 'var(--kh-space-6)', textAlign: 'center', color }, children: [_jsx("p", { style: { margin: 0 }, children: text }), kind === 'error' && onRetry ? (_jsx("div", { style: { marginTop: 'var(--kh-space-3)' }, children: _jsx(Button, { variant: "ghost", onClick: onRetry, children: "\u062A\u0644\u0627\u0634 \u062F\u0648\u0628\u0627\u0631\u0647" }) })) : null] }));
}
// useAsync is the one loading primitive: it tracks loading/error/data and exposes a
// retry, so no screen has to re-implement the three states.
export function useAsync(fn, deps = []) {
    const [state, setState] = useState({
        loading: true,
        error: null,
        data: null,
    });
    const [tick, setTick] = useState(0);
    useEffect(() => {
        let alive = true;
        setState((s) => ({ ...s, loading: true, error: null }));
        fn()
            .then((data) => alive && setState({ loading: false, error: null, data }))
            .catch((err) => alive && setState({ loading: false, error: err instanceof Error ? err.message : 'خطای نامشخص', data: null }));
        return () => {
            alive = false;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [...deps, tick]);
    return { ...state, retry: () => setTick((t) => t + 1) };
}
// ── layout chrome ──
export function Header({ items }) {
    return (_jsx("header", { className: "kh-dark", style: {
            position: 'sticky',
            top: 0,
            zIndex: 10,
            background: 'var(--kh-bg)',
            borderBottom: '1px solid var(--kh-border)',
            height: 'var(--kh-header-h)',
            display: 'flex',
            alignItems: 'center',
        }, children: _jsx(Container, { children: _jsxs("nav", { style: { display: 'flex', alignItems: 'center', gap: 'var(--kh-space-5)', height: 'var(--kh-header-h)' }, children: [_jsx("a", { href: "#/", style: { fontWeight: 700, color: 'var(--kh-text)', fontSize: 'var(--kh-text-lg)' }, children: "\u062E\u0648\u0627\u0628\u200C\u0622\u0648\u0631" }), _jsx("ul", { style: { display: 'flex', gap: 'var(--kh-space-4)', listStyle: 'none', margin: 0, padding: 0, flexWrap: 'wrap' }, children: items.map((it) => (_jsx("li", { children: _jsx("a", { href: it.href, style: { color: 'var(--kh-text-muted)' }, children: it.label }) }, it.href))) })] }) }) }));
}
export function Footer() {
    return (_jsx("footer", { className: "kh-dark", style: { background: 'var(--kh-bg)', borderTop: '1px solid var(--kh-border)', marginTop: 'var(--kh-space-7)' }, children: _jsx(Container, { children: _jsxs("div", { style: { padding: 'var(--kh-space-6) 0', color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }, children: [_jsx("p", { style: { margin: 0 }, children: "\u062E\u0648\u0627\u0628\u200C\u0622\u0648\u0631 \u2014 \u0627\u06A9\u0648\u0633\u06CC\u0633\u062A\u0645 \u062A\u062C\u0627\u0631\u062A \u0648 \u0645\u062D\u062A\u0648\u0627\u06CC \u062A\u062E\u0635\u0635\u06CC \u062E\u0648\u0627\u0628" }), _jsx("p", { style: { margin: 'var(--kh-space-2) 0 0' }, children: "\u0645\u062D\u062A\u0648\u0627\u06CC \u0645\u062C\u0644\u0647 \u062C\u0627\u06CC\u06AF\u0632\u06CC\u0646 \u062A\u0648\u0635\u06CC\u0647 \u067E\u0632\u0634\u06A9\u06CC \u0646\u06CC\u0633\u062A." })] }) }) }));
}
// ── domain cards ──
export function ProductCard({ product, href }) {
    return (_jsxs(Card, { children: [_jsx("h3", { style: { margin: 0 }, children: _jsx("a", { href: href, style: { color: 'var(--kh-text)' }, children: product.title }) }), product.short_description ? (_jsx("p", { style: { margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }, children: product.short_description })) : null, product.sleep_relevance ? _jsx(Badge, { tone: "success", children: product.sleep_relevance }) : null] }));
}
export function ArticleCard({ article, href }) {
    return (_jsxs(Card, { children: [_jsx("h3", { style: { margin: 0 }, children: _jsx("a", { href: href, style: { color: 'var(--kh-text)' }, children: article.title }) }), article.summary ? (_jsx("p", { style: { margin: 0, color: 'var(--kh-text-muted)', fontSize: 'var(--kh-text-sm)' }, children: article.summary })) : null] }));
}
// Grid is the one layout primitive: responsive without media-query duplication.
export function Grid({ children, min = '260px' }) {
    return (_jsx("div", { style: { display: 'grid', gap: 'var(--kh-space-4)', gridTemplateColumns: `repeat(auto-fill, minmax(${min}, 1fr))` }, children: children }));
}
