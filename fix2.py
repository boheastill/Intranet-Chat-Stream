import re

text = open('static/index.txt', 'r', encoding='utf-8', errors='ignore').read()
text = re.sub(r'主频\?\(默认\)', '主频道(默认)', text)
text = re.sub(r'<option value="">[^<]*</option>', '<option value="">主频道(默认)</option>', text)
text = re.sub(r'<option value="lab">[^<]*</option>', '<option value="lab">代码实验(Lab)</option>', text)

text = text.replace('主频?', '主频道')
text = text.replace('?(Lab)', '实验(Lab)')
text = text.replace('代码实?(Lab)', '代码实验(Lab)')
text = text.replace('代码实验?', '代码实验')

text = re.sub(r'title="[^"]*?>', 'title="退出登录">', text)
text = text.replace('title="退出?', 'title="退出登录">')
text = text.replace('title="?', 'title="退出登录">')

text = text.replace('暂无消息流。您可以粘贴剪贴板文字或拖入文件开始?', '暂无消息流。您可以粘贴剪贴板文字或拖入文件开始。</p>')
text = text.replace('拖放至此安全投递?', '拖放至此安全投递</p>')
text = text.replace('输入或粘贴文字 (Ctrl + Enter 投递)?', '输入或粘贴文字 (Ctrl + Enter 投递)...')
text = text.replace('投?/span>', '投递</span>')
text = text.replace('需要身份验证?', '需要身份验证')
text = text.replace('请输入您的安全 Token 或密保密码?', '请输入您的安全 Token 或密保密码')

# Also fix the weird unicode chars from the screenshot
text = text.replace('', '')

with open('static/index.html', 'w', encoding='utf-8') as f:
    f.write(text)
print('Done!')
