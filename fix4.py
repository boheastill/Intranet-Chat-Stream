text = open('static/index.txt', 'r', encoding='utf-8', errors='ignore').read()
lines = text.split('\n')

# UI fixes
lines[646] = '                <option value="">主频道(默认)</option>'
lines[647] = '                <option value="upwork">海外接单 (Upwork)</option>'
lines[648] = '                <option value="syslog">系统运行日志 (SysLog)</option>'
lines[649] = '                <option value="lab">代码实验(Lab)</option>'
lines[654] = '            <span id="statusText">正在同步...</span>'
lines[655] = '            <button id="btnLogoutHeader" style="background: transparent; border: none; color: var(--text-secondary); cursor: pointer; margin-left: 0.75rem; display: inline-flex; align-items: center; justify-content: center; outline: none; transition: color 0.2s;" title="退出登录">'
lines[671] = '                <p>暂无消息流。您可以粘贴剪贴板文字或拖入文件开始。</p>'
lines[678] = '                <h3 style="color:#818cf8; margin-bottom: 0.5rem;">松开以上传文件</h3>'
lines[679] = '                <p style="color:var(--text-secondary); font-size: 0.9rem;">拖放至此安全投递</p>'
lines[687] = '                    <textarea id="textInput" class="text-textarea" placeholder="输入或粘贴文字 (Ctrl + Enter 投递)..."></textarea>'
lines[703] = '                            <span>投递</span>'
lines[728] = '            <h2 id="loginTitle" style="font-size: 1.4rem; font-weight: 600; background: linear-gradient(to right, #ffffff, var(--text-secondary)); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">需要身份验证</h2>'
lines[729] = '            <p id="loginSubtitle" style="color: var(--text-secondary); font-size: 0.9rem; line-height: 1.4;">请输入您的安全 Token 或密保密码</p>'

# JS fixes
lines[864] = "                loginSubtitle.innerText = '通过密保密码验证';"
lines[870] = "                loginTitle.innerText = '需要身份验证';"
lines[871] = "                loginSubtitle.innerText = '请输入您的安全 Token 或密保密码';"
lines[917] = "                        throw new Error('未提供安全 Token');"
lines[924] = "                    btnLogin.innerText = '验证中...';"
lines[1024] = "                        <p>暂无消息流。您可以粘贴剪贴板文字或拖入文件开始。</p>"
lines[1093] = "                            tip.innerText = '已复制';"
lines[1152] = "                            copyBtn.innerHTML = '<span>已复制</span>';"
lines[1185] = "                    if (msg.content.includes('[内容过长已截断，请下载完整文件]')) {"
lines[1278] = "                    statusText.innerText = `上传中... ${percent}%`;"
lines[1302] = "                    alert('投递失败: ' + xhr.responseText);"
lines[1310] = "                alert('文件投递过程中发生网络错误。');"

with open('static/index.html', 'w', encoding='utf-8') as f:
    f.write('\n'.join(lines))
print('Perfect fix applied!')
