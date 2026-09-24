#!/usr/bin/env python3
"""Serve testdata/app-live with the same routing as liveutil.ServeApp:
extensionless paths map to <name>.html, "/" maps to home.html.
Used for the site/bench.json pin measurement (greedy run --base-url).

  python3 scripts/serve-app-live.py [--port 3030]
"""
from __future__ import annotations

import argparse
import http.server
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
APP = ROOT / "testdata" / "app-live"


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):  # quiet
        pass

    def do_GET(self):
        path = self.path.split("?", 1)[0].strip("/") or "home"
        target = APP / (path if path.endswith(".html") else f"{path}.html")
        if not target.is_file():
            self.send_error(404)
            return
        body = target.read_bytes()
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--port", type=int, default=3030)
    args = p.parse_args()
    srv = http.server.ThreadingHTTPServer(("0.0.0.0", args.port), Handler)
    print(f"serving {APP} on http://localhost:{args.port}/")
    srv.serve_forever()


if __name__ == "__main__":
    raise SystemExit(main())
