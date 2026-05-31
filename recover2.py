import json
import os

log_path = r'C:\Users\bohea\.gemini\antigravity\brain\2e11ebfa-5bff-4555-830b-cb0f3e25c2cd\.system_generated\logs\transcript.jsonl'
html = ''

if not os.path.exists(log_path):
    print("Log not found:", log_path)
else:
    for line in open(log_path, 'r', encoding='utf-8'):
        try:
            obj = json.loads(line)
            if 'tool_calls' in obj and len(obj['tool_calls']) > 0:
                for tc in obj['tool_calls']:
                    if tc['function'] == 'default_api:write_to_file':
                        args = tc.get('arguments', {})
                        if 'index.html' in args.get('TargetFile', ''):
                            html = args.get('CodeContent', '')
        except Exception:
            pass

    if html:
        with open('static/index.html', 'w', encoding='utf-8') as f:
            f.write(html)
        print('Restored successfully, length:', len(html))
    else:
        print('Failed to find index.html in logs')
