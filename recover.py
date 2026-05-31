import json
log_path = r'C:\Users\bohea\.gemini\antigravity\brain\b19d255a-e053-47bb-8d78-a098ceacd610\.system_generated\logs\transcript.jsonl'
html = ''
for line in open(log_path, 'r', encoding='utf-8'):
    try:
        obj = json.loads(line)
        if 'tool_calls' in obj and len(obj['tool_calls']) > 0:
            tc = obj['tool_calls'][0]
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
