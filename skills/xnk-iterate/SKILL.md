---
name: xnk-iterate
description: >-
  Bind the current Google Antigravity conversation to XNK Iterate (running locally at
  http://127.0.0.1:9292) so live voice commentary, dynamic lookback timestamps, OS interactions,
  and Trigger/Reaction screen snapshots stream directly into this session. Trigger when the user
  runs "/xnk-iterate" or asks to link/connect XNK Iterate.
---

# XNK Iterate — Live Voice + Vision Partner Skill

When the user invokes `/xnk-iterate` (or asks to connect XNK Iterate to this session):

1. **Read Your Conversation ID** from `<user_information>` (`Conversation ID: <id>`).
2. **Bind to XNK Iterate** by posting your Conversation ID and Antigravity's local Language Server connection variables (`ANTIGRAVITY_LS_ADDRESS`, `ANTIGRAVITY_CSRF_TOKEN`, `ANTIGRAVITY_AGENTAPI_EXE`) to the local XNK Iterate companion bridge (run with `BypassSandbox: true` so loopback TCP reaches `127.0.0.1:9292`):
   ```bash
   python3 -c 'import json, os, urllib.request
payload = json.dumps({
  "conversation_id": os.environ.get("ANTIGRAVITY_CONVERSATION_ID") or "<YOUR_CONVERSATION_ID>",
  "ls_address": os.environ.get("ANTIGRAVITY_LS_ADDRESS", ""),
  "csrf_token": os.environ.get("ANTIGRAVITY_CSRF_TOKEN", ""),
  "agentapi_exe": os.environ.get("ANTIGRAVITY_AGENTAPI_EXE", ""),
  "app_data_dir": os.environ.get("ANTIGRAVITY_APP_DATA_DIR", ""),
}).encode()
req = urllib.request.Request("http://127.0.0.1:9292/api/bind", data=payload, headers={"Content-Type": "application/json"})
print(urllib.request.urlopen(req, timeout=5).read().decode())'
   ```
   *(If `127.0.0.1:9292` is unreachable, let the user know to launch `XNK Iterate.app` first.)*
3. **Confirm the Link** in 1 short sentence (including the linked repo/status returned by `/api/bind`) and tell the user they can press **Start** in XNK Iterate's HUD, talk naturally, and press their Send hotkey (default `⌥↩`) whenever they want to send a turn.
4. **When XNK Iterate Director Messages Arrive:**
   - Immediately inspect any attached `[trigger](...)`, `[reaction](...)`, or `[frame N](...)` image paths using `read_file` to see exactly what happened on screen.
   - Cross-reference the lookback timestamps and any click/key coordinates in the footer pill with the codebase.
   - Keep replies concise and action-oriented while the user continues playtesting!
