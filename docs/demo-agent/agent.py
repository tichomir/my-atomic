#!/usr/bin/python3
"""
Atomic OS demo agent — minimal HTTP chat agent.

Register:
  atomic-agent-ctl register --id logo-finder --profile developer \
    --exec /var/lib/atomic/agents/logo-finder/agent.py

Interact:
  curl -s http://localhost:8888/health
  curl -s -X POST http://localhost:8888/chat \
    -H 'Content-Type: application/json' \
    -d '{"message": "what files are in my workspace?"}'
"""

import http.server
import json
import os
import urllib.request
import urllib.error

AGENT_ID  = os.environ.get("ATOMIC_AGENT_ID", "unknown")
PORT      = int(os.environ.get("AGENT_PORT", "8888"))
WORKSPACE = os.environ.get("ATOMIC_WORKSPACE",
            f"/var/lib/atomic/agents/{AGENT_ID}/workspace")

# Load secrets from <agent-root>/env (key=value, one per line).
# Keeps the API key out of the daemon API and the audit log.
_env_file = f"/var/lib/atomic/agents/{AGENT_ID}/env"
if os.path.exists(_env_file):
    with open(_env_file) as _f:
        for _line in _f:
            _line = _line.strip()
            if _line and not _line.startswith("#") and "=" in _line:
                _k, _v = _line.split("=", 1)
                os.environ.setdefault(_k.strip(), _v.strip())

API_KEY = os.environ.get("ANTHROPIC_API_KEY", "")

SYSTEM_PROMPT = f"""You are a helpful AI agent running inside Atomic OS.
Your agent ID is {AGENT_ID}.
Your workspace (the only directory you may write to) is {WORKSPACE}.
Be concise and helpful."""


def call_claude(message: str) -> str:
    payload = json.dumps({
        "model": "claude-haiku-4-5-20251001",
        "max_tokens": 512,
        "system": SYSTEM_PROMPT,
        "messages": [{"role": "user", "content": message}],
    }).encode()
    req = urllib.request.Request(
        "https://api.anthropic.com/v1/messages",
        data=payload,
        headers={
            "x-api-key": API_KEY,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        },
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read())["content"][0]["text"]


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"status": "ok", "agent_id": AGENT_ID,
                             "profile": os.environ.get("ATOMIC_AGENT_PROFILE", "?")})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/chat":
            self._json(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", 0))
            body   = json.loads(self.rfile.read(length))
            msg    = body.get("message", "").strip()
            if not msg:
                self._json(400, {"error": "missing message"})
                return
            if not API_KEY:
                self._json(503, {"error": "ANTHROPIC_API_KEY not set"})
                return
            self._json(200, {"reply": call_claude(msg), "agent_id": AGENT_ID})
        except urllib.error.HTTPError as e:
            self._json(502, {"error": f"upstream API error: {e.code}"})
        except Exception as e:
            self._json(500, {"error": str(e)})

    def _json(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        print(f"[{self.address_string()}] {fmt % args}", flush=True)


if __name__ == "__main__":
    print(f"[agent:{AGENT_ID}] workspace={WORKSPACE}", flush=True)
    print(f"[agent:{AGENT_ID}] listening on :{PORT}", flush=True)
    server = http.server.ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    server.serve_forever()
