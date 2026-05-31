import json
import glob
import os

html = None
for path in glob.glob(r'C:\Users\bohea\.gemini\antigravity\brain\*\.system_generated\logs\transcript.jsonl'):
    try:
        with open(path, 'r', encoding='utf-8') as f:
            for line in f:
                if 'Intranet Chat Stream' in line and '<!DOCTYPE html>' in line:
                    # Parse JSON
                    try:
                        obj = json.loads(line)
                        if 'tool_calls' in obj:
                            for tc in obj['tool_calls']:
                                if 'index.html' in str(tc):
                                    args = tc.get('arguments', {})
                                    content = args.get('CodeContent', '') or args.get('ReplacementContent', '')
                                    if '<title>Intranet Chat Stream</title>' in content:
                                        html = content
                                        break
                    except Exception:
                        pass
                if html:
                    break
    except Exception:
        pass
    if html:
        break

if html:
    with open('static/index.html', 'w', encoding='utf-8') as f:
        f.write(html)
    print(f'Successfully recovered original index.html! Length: {len(html)}')
else:
    print('Failed to find original index.html')
