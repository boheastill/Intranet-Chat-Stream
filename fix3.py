import re

text = open('static/index.txt', 'r', encoding='utf-8', errors='ignore').read()

# Fix the drag and drop text
text = re.sub(r'<p style="color:var\(--text-secondary\); font-size: 0\.9rem;">.*?</p>', '<p style="color:var(--text-secondary); font-size: 0.9rem;">拖放至此安全投递</p>', text)

# Fix the textarea
text = re.sub(r'<textarea id="textInput" class="text-textarea" placeholder=".*?"></textarea>', '<textarea id="textInput" class="text-textarea" placeholder="输入或粘贴文字 (Ctrl + Enter 投递)..."></textarea>', text)

# Fix the submit button span
text = re.sub(r'<button id="btnSubmit" class="btn-primary" style="padding: 0\.5rem 1\.25rem;">\s*<span>.*?</span>', '<button id="btnSubmit" class="btn-primary" style="padding: 0.5rem 1.25rem;">\n                                <span>投递</span>', text, flags=re.DOTALL)

# Fix the login title
text = re.sub(r'<h2 id="loginTitle".*?>.*?</h2>', '<h2 id="loginTitle" style="font-size: 1.4rem; font-weight: 600; background: linear-gradient(to right, #ffffff, var(--text-secondary)); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">需要身份验证</h2>', text, flags=re.DOTALL)

# Fix the login subtitle
text = re.sub(r'<p id="loginSubtitle".*?>.*?</p>', '<p id="loginSubtitle" style="color: var(--text-secondary); font-size: 0.9rem; line-height: 1.4;">请输入您的安全 Token 或密保密码</p>', text, flags=re.DOTALL)

with open('static/index.html', 'w', encoding='utf-8') as f:
    f.write(text)
print('Regex fixed!')
