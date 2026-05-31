import re

with open('static/index.txt', 'r', encoding='utf-8', errors='ignore') as f:
    text = f.read()

# Fix broken strings
text = re.sub(r'<option value="">[^<]*</option>', '<option value="">主频道(默认)</option>', text)
text = re.sub(r'<option value="lab">[^<]*</option>', '<option value="lab">代码实验(Lab)</option>', text)
text = re.sub(r'title="[^"]*?>', 'title="退出登录">', text)
text = re.sub(r'<p>暂无消息流[^<]*</p>', '<p>暂无消息流。您可以粘贴剪贴板文字或拖入文件开始。</p>', text)
text = re.sub(r'<p style="color:var\(--text-secondary\); font-size: 0\.9rem;">[^<]*</p>', '<p style="color:var(--text-secondary); font-size: 0.9rem;">拖放至此安全投递</p>', text)
text = re.sub(r'<textarea id="textInput" class="text-textarea" placeholder="[^"]*"></textarea>', '<textarea id="textInput" class="text-textarea" placeholder="输入或粘贴文字 (Ctrl + Enter 投递)..."></textarea>', text)
text = re.sub(r'<span>[^<]*</span>', '<span>投递</span>', text)
text = re.sub(r'<h2 id="loginTitle"[^>]*>[^<]*</h2>', '<h2 id="loginTitle" style="font-size: 1.4rem; font-weight: 600; background: linear-gradient(to right, #ffffff, var(--text-secondary)); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">需要身份验证</h2>', text)
text = re.sub(r'<p id="loginSubtitle"[^>]*>[^<]*</p>', '<p id="loginSubtitle" style="color: var(--text-secondary); font-size: 0.9rem; line-height: 1.4;">请输入您的安全 Token 或密保密码</p>', text)

with open('static/index.html', 'w', encoding='utf-8') as f:
    f.write(text)

print('Fixed UI strings!')
