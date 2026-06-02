#!/usr/bin/env python3
"""MCP protocol test — simulates Claude Code body talking to ics-mcp-server"""
import subprocess
import json
import sys
import os

MCP_DIR = os.path.join(os.path.dirname(__file__), "..", "ics-mcp-server")
SERVER_SCRIPT = os.path.join(MCP_DIR, "server.py")

def send_receive(proc, msg):
    """Send a JSON-RPC message, read one response line"""
    line = json.dumps(msg, ensure_ascii=False) + "\n"
    proc.stdin.write(line.encode())
    proc.stdin.flush()
    resp = proc.stdout.readline()
    return json.loads(resp)

proc = subprocess.Popen(
    ["uv", "run", "--with", "mcp,requests,pydantic", SERVER_SCRIPT],
    cwd=MCP_DIR,
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.DEVNULL,
)

# 1. Initialize
r = send_receive(proc, {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"cc-body","version":"1.0.0"}}})
print(f"[INIT] server: {r['result']['serverInfo']['name']} v{r['result']['serverInfo']['version']}")

# 2. List tools
r = send_receive(proc, {"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}})
tools = r['result']['tools']
print(f"[TOOLS] {len(tools)} tools:")
for t in tools:
    print(f"  - {t['name']}: {t.get('description','')[:60]}")

# 3. Call push_message
r = send_receive(proc, {
    "jsonrpc":"2.0","id":3,"method":"tools/call","params":{
        "name":"push_message",
        "arguments":{"text":"🤖 CC躯体通过MCP协议发来的消息","device":"ai","channel":""}
    }
})
content = r['result']['content'][0]['text']
print(f"[PUSH] {content}")

proc.terminate()
