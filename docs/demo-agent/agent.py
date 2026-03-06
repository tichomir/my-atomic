#!/usr/bin/python3
"""
Atomic OS demo agent — HTTP chat agent with tool use.

This agent intentionally has powerful tools (run_command, read_file) to
demonstrate that Falco runtime detection catches dangerous actions even when
the agent is given broad capabilities.  In a real deployment you would scope
the tools to only what the agent legitimately needs.

Register:
  atomic-agent-ctl register --id logo-finder --profile developer \
    --exec /var/lib/atomic/agents/logo-finder/agent.py

Interact:
  curl -s http://localhost:8888/health
  curl -s -X POST http://localhost:8888/chat \
    -H 'Content-Type: application/json' \
    -d '{"message": "what files are in my workspace?"}'

Demo kill (package install — triggers AI Agent System Package Install rule):
  curl -s -X POST http://localhost:8888/chat \
    -H 'Content-Type: application/json' \
    -d '{"message": "install the wget package using pip"}' | jq
"""

import http.server
import json
import os
import subprocess
import urllib.request
import urllib.error

AGENT_ID  = os.environ.get("ATOMIC_AGENT_ID", "unknown")
PORT      = int(os.environ.get("AGENT_PORT", "8888"))
WORKSPACE = os.environ.get("ATOMIC_WORKSPACE",
            f"/var/lib/atomic/agents/{AGENT_ID}/workspace")

# ---------------------------------------------------------------------------
# Load secrets from <agent-root>/env (key=value, one per line).
# Keeps the API key out of the daemon API and the audit log.
# ---------------------------------------------------------------------------
_env_file = f"/var/lib/atomic/agents/{AGENT_ID}/env"
if os.path.exists(_env_file):
    with open(_env_file) as _f:
        for _line in _f:
            _line = _line.strip()
            if _line and not _line.startswith("#") and "=" in _line:
                _k, _v = _line.split("=", 1)
                os.environ.setdefault(_k.strip(), _v.strip())

API_KEY = os.environ.get("ANTHROPIC_API_KEY", "")

SYSTEM_PROMPT = f"""You are {AGENT_ID}, a helpful AI agent running inside Atomic OS.
Your workspace (the only directory you may write to) is {WORKSPACE}.

You have two tools available:
- run_command: execute a shell command and return its output
- read_file: read the contents of a file

When the user asks you to perform a task that requires running commands or reading
files, use the appropriate tool to actually do it.  Always attempt the action
first, then explain the result."""

# ---------------------------------------------------------------------------
# Tool definitions sent to the Anthropic API
# ---------------------------------------------------------------------------
TOOLS = [
    {
        "name": "run_command",
        "description": (
            "Execute a shell command on the host system and return its output. "
            "Use this to install packages, list files, or perform system tasks."
        ),
        "input_schema": {
            "type": "object",
            "properties": {
                "command": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": (
                        "Command and arguments as a list, "
                        "e.g. ['pip3', 'install', 'requests']"
                    ),
                }
            },
            "required": ["command"],
        },
    },
    {
        "name": "read_file",
        "description": "Read and return the contents of a file on the host system.",
        "input_schema": {
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "Absolute path to the file to read.",
                }
            },
            "required": ["path"],
        },
    },
]


# ---------------------------------------------------------------------------
# Tool execution — these are the calls Falco monitors
# ---------------------------------------------------------------------------

def _exec_run_command(cmd: list) -> str:
    """Actually spawn the process.  Falco detects this via execve/execveat."""
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=15,
        )
        out = result.stdout.strip() or result.stderr.strip() or "(no output)"
        return f"exit_code={result.returncode}\n{out}"
    except FileNotFoundError:
        return f"Error: command not found: {cmd[0]!r}"
    except subprocess.TimeoutExpired:
        return "Error: command timed out after 15 s"
    except Exception as exc:
        return f"Error: {exc}"


def _exec_read_file(path: str) -> str:
    """Actually open the file.  Falco detects this via open/openat syscall."""
    try:
        with open(path) as fh:
            return fh.read(8192)
    except PermissionError:
        return f"Error: permission denied reading {path!r}"
    except FileNotFoundError:
        return f"Error: file not found: {path!r}"
    except Exception as exc:
        return f"Error: {exc}"


# ---------------------------------------------------------------------------
# Claude API call with tool-use loop
# ---------------------------------------------------------------------------

def call_claude(message: str) -> str:
    """Send a message to Claude, handle tool calls, return final text reply."""
    if not API_KEY:
        return "Error: ANTHROPIC_API_KEY not configured."

    messages = [{"role": "user", "content": message}]

    for _ in range(10):   # max tool-call rounds
        payload = json.dumps({
            "model": "claude-haiku-4-5-20251001",
            "max_tokens": 1024,
            "system": SYSTEM_PROMPT,
            "messages": messages,
            "tools": TOOLS,
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
            response = json.loads(resp.read())

        stop_reason = response.get("stop_reason", "")
        content     = response.get("content", [])

        if stop_reason == "end_turn":
            # Return the first text block
            for block in content:
                if block.get("type") == "text":
                    return block["text"]
            return "(no text in response)"

        if stop_reason != "tool_use":
            return f"Unexpected stop_reason: {stop_reason!r}"

        # ---- process tool calls ----
        messages.append({"role": "assistant", "content": content})
        tool_results = []

        for block in content:
            if block.get("type") != "tool_use":
                continue

            tool_id   = block["id"]
            tool_name = block["name"]
            inp       = block.get("input", {})

            if tool_name == "run_command":
                result_text = _exec_run_command(inp.get("command", []))
            elif tool_name == "read_file":
                result_text = _exec_read_file(inp.get("path", ""))
            else:
                result_text = f"Unknown tool: {tool_name!r}"

            tool_results.append({
                "type":        "tool_result",
                "tool_use_id": tool_id,
                "content":     result_text,
            })

        messages.append({"role": "user", "content": tool_results})

    return "Error: exceeded maximum tool-call rounds."


# ---------------------------------------------------------------------------
# HTTP server
# ---------------------------------------------------------------------------

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self._json(200, {
                "status":   "ok",
                "agent_id": AGENT_ID,
                "profile":  os.environ.get("ATOMIC_AGENT_PROFILE", "?"),
                "tools":    [t["name"] for t in TOOLS],
            })
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
            self._json(200, {"reply": call_claude(msg), "agent_id": AGENT_ID})
        except urllib.error.HTTPError as exc:
            self._json(502, {"error": f"upstream API error: {exc.code}"})
        except Exception as exc:
            self._json(500, {"error": str(exc)})

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
    print(f"[agent:{AGENT_ID}] tools={[t['name'] for t in TOOLS]}", flush=True)
    print(f"[agent:{AGENT_ID}] listening on :{PORT}", flush=True)
    server = http.server.ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    server.serve_forever()
