#!/usr/bin/env python3
"""Serve a built SPA with history fallback, so client routes resolve.

Why this exists: `python3 -m http.server` answers 404 for /auth/callback, /orders, anything the
router owns. The admin panel's OIDC flow lands on /auth/callback, so a local preview could never
finish a login — it ended on a blank 404 page. Production nginx already does
`try_files $uri $uri/ /index.html` (deploy/nginx/khabavar.conf), so this is the local equivalent,
not a product change.

Usage:
  python3 scripts/serve-spa.py web/admin/dist 5193
"""
import http.server
import os
import socketserver
import sys


class SPAHandler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *a, directory=None, **kw):
        super().__init__(*a, directory=directory, **kw)

    def do_GET(self):
        path = self.translate_path(self.path)
        # A real file wins; anything else is a client route and gets index.html.
        if not os.path.exists(path) or os.path.isdir(path):
            if not os.path.exists(os.path.join(self.directory, "index.html")):
                self.send_error(404, "no index.html in the dist directory")
                return
            self.path = "/index.html"
        return super().do_GET()

    def log_message(self, fmt, *args):
        sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        return 2
    root, port = sys.argv[1], int(sys.argv[2])
    if not os.path.isdir(root):
        print(f"not a directory: {root}")
        return 2
    handler = lambda *a, **kw: SPAHandler(*a, directory=root, **kw)  # noqa: E731
    socketserver.TCPServer.allow_reuse_address = True
    with socketserver.TCPServer(("127.0.0.1", port), handler) as httpd:
        print(f"serving {root} on http://127.0.0.1:{port} (SPA fallback on)")
        httpd.serve_forever()


if __name__ == "__main__":
    sys.exit(main())
