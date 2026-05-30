# Intranet Chat Stream (ICS) - Core

ICS-Core 是一个超轻量级、零数据库（DB-Less）、面向“人机协同（HITL）”与“跨设备数据流”设计的极简剪贴板及文件中转站。

本仓库实现了 **ICS-Core（数据面 / 哑管道）** 的 Go 语言核心后端，以及对应的磨砂玻璃现代 Web 前端。

---

## 1. 核心架构哲学 (Dumb Pipe & Smart Consumers)

*   **ICS-Core (哑管道 / Go)**：定位为无状态、高并发、高稳定的数据流通道，只负责文本/文件存盘、检索及空间滚动清理。
*   **Consumers (智能消费者 / Python/JS)**：包括您的手机/电脑 Web 前端、本地 AI 自动化 Agent（如任务抓取、通知推送脚本）。它们通过标准的 API Header Token 进行鉴权消费，随时重构，与核心服务彻底解耦。

---

## 2. 目录布局 (Directory Layout)

```text
ssh-connect/
├── main.go               # Go 后端路由与核心业务逻辑（自包含，零三方库依赖）
├── static/
│   └── index.html        # 单页面现代暗黑 Web 前端（磨砂玻璃拟态设计）
├── files/                # 物理存储剪贴板历史文件和图片（运行自动创建，git已忽略）
├── config.json           # 存储您的静态访问密钥 Token（运行自动生成，git已忽略）
├── .gitignore            # 排除编译目标及本地状态数据
└── README.md             # 本说明文档
```

---

## 3. 安全防护体系 (Security Model)

*   **零入站防火墙 (Cloudflare Tunnel)**：VPS 本地服务仅监听 `127.0.0.1:6666`。外部攻击者及全网扫描器无法从公网直接探测该端口。所有请求均由 VPS 主动出站连接 Cloudflare 隧道进行加密中转。
*   **应用层 Token 验证 (Header / Query)**：
    *   API 接口均需验证 `X-Auth-Token` 头部。
    *   图片及二进制文件下载链接支持从 URL 参数中校验 `?token=`。
    *   未认证访问直接返回 `401 Unauthorized` 阻断。
*   **路径穿透清洗**：所有文件读写动作均执行严格的 `filepath` 安全防线校验，杜绝 `../` 等黑客恶意文件读取注入。

---

## 4. 本地构建与运行 (Local Setup)

### 4.1 编译与启动
在 Go 安装就绪的环境下，直接执行：
```bash
# 启动本地开发服务
go run main.go
```
首次运行会在项目目录下生成 `config.json`，并打印系统自动产生的 32 位安全 Token。

### 4.2 访问系统
*   浏览器打开 `http://127.0.0.1:6666`。
*   根据提示输入终端打印的 Token，即可开始使用。

---

## 5. 生产环境编译与 VPS 部署 (Debian/Ubuntu)

### 5.1 交叉编译 (Linux amd64)
在 Windows 开发环境下，执行以下命令编译为 Linux 下的 ELF 二进制可执行文件：
```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o clipstream
```

### 5.2 部署目录配置
通过 SCP/SFTP 将编译好的 `clipstream` 二进制文件与 `static/` 文件夹上传到 VPS 的 `/home/admin/clipstream/` 目录下。

### 5.3 配置守护进程 (systemd)
创建 `/etc/systemd/system/clipstream.service`：
```ini
[Unit]
Description=Intranet Chat Stream (ICS) Core Service
After=network.target

[Service]
Type=simple
User=admin
WorkingDirectory=/home/admin/clipstream
ExecStart=/home/admin/clipstream/clipstream
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```
启动并启用自启：
```bash
sudo systemctl daemon-reload
sudo systemctl enable clipstream --now
```

---

## 6. API 协议规范 (API Contract)

### 6.1 `GET /api/list` (拉取消息流)
*   **鉴权 Header**：`X-Auth-Token: [您的密钥]`
*   **响应示例** (按时间戳倒序)：
    ```json
    [
      {
        "id": "1780167900_text.txt",
        "type": "text",
        "content": "验证码: 9981",
        "time": "2026-05-31 03:05:00",
        "pinned": false
      }
    ]
    ```

### 6.2 `POST /api/push` (投送文本/文件)
*   **鉴权 Header**：`X-Auth-Token: [您的密钥]`
*   **请求体** (Multipart Form)：
    *   `text` (可选，String)：文本框输入
    *   `file` (可选，File)：上传的文件
*   **响应示例**：
    ```json
    {"id":"1780167900_text.txt","status":"success"}
    ```

### 6.3 `POST /api/action` (锁定置顶/删除卡片)
*   **鉴权 Header**：`X-Auth-Token: [您的密钥]`
*   **请求体** (JSON)：
    ```json
    {"id": "1780167900_text.txt", "action": "pin"} 
    // 支持 action: "pin" (置顶保护), "unpin" (取消置顶), "delete" (物理删除)
    ```

---

## 7. Python AI Agent (ICS-Agent) 对接模板

```python
import requests

TOKEN = "006e91db7de25746bd029895f9e5f45b"  # 替换为 config.json 里的真实 Token
BASE_URL = "https://flow.bohea.us"

headers = {
    "X-Auth-Token": TOKEN
}

# 1. 拉取当前待处理流
res = requests.get(f"{BASE_URL}/api/list", headers=headers)
messages = res.json()
print("当前流中消息数量:", len(messages))

# 2. 推送处理完毕的消息
payload = {
    "text": "🤖 AI Agent 分析完毕：待提取验证码为 [9981]"
}
res_push = requests.post(f"{BASE_URL}/api/push", headers=headers, data=payload)
print("推送结果:", res_push.json())
```
