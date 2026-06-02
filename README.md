# Intranet Chat Stream (ICS) - Core

ICS-Core 是一个超轻量级、零数据库（DB-Less）、面向“人机协同（HITL）”与“跨设备数据流”设计的极简剪贴板及文件中转站。

本仓库实现了 **ICS-Core（数据面 / 哑管道）** 的 Go 语言核心后端，以及对应的磨砂玻璃现代 Web 前端。

---

## 1. 核心架构哲学 (Dumb Pipe & Smart Consumers)

*   **ICS-Core (哑管道 / Go)**：定位为无状态、高并发、高稳定的数据流通道，只负责文本/文件存盘、检索、磁盘容量统计及空间滚动清理。
    *   **大文本截断防御**：获取列表时，文本文件若超过 10KB 自动截断并追加提示，保障长文本频繁轮询时的无感 I/O 和极致首屏加载性能。
    *   **极简审计日志**：对成功的写入、操作和系统级淘汰，通过标准输出流（Stdout / systemd journal）打印无噪音审计日志。
*   **Web 客户端 (HTML5/JS)**：提供极致阅读器体验的单页应用。
    *   **亮色纸质护眼主题**：全天候护眼配色，自适应设备高度（防软键盘遮挡），实现 100dvh 无缝体验。
    *   **多文件无缝秒传**：深度整合系统文件管理器，支持多文件并发选择并自动上传，告别多余的“确认”点击。
*   **Consumers (智能消费者 / Python/JS)**：您的本地 AI 自动化 Agent（如任务抓取、通知推送脚本）。通过标准的 API Header Token 进行鉴权消费，与核心服务彻底解耦。

---

## 2. 目录布局 (Directory Layout)

```text
ics-core/                 # Go module `ics`
├── main.go               # 启动装配：加载配置 → 启动 pipeline → 运行 bus
├── bus/                  # Message Bus（核心）：REST + SSE HTTP 服务、文件存储、鉴权
│   ├── server.go         #   Run/路由/中间件/静态文件
│   ├── handlers.go       #   list/push/action/stream/download/login
│   ├── store.go          #   文件存储与文件名解析、滚动清理
│   ├── config.go         #   Config 加载/生成
│   ├── broadcaster.go    #   SSE 广播
│   └── auth.go           #   Token 中间件 + 登录指数退避
├── pipeline/             # Pipeline（核心）：SSE 消费 → 触发词路由 → AI → 回复
│   └── pipeline.go
├── ai/                   # AI Backend 接口 + Template + DeepSeek + MiMo + router(触发词路由表)
├── knowledge/            # 文件级知识库（关键词检索）
├── static/
│   └── index.html        # 单页面现代 Web 前端（磨砂玻璃 + 设备检测 + 登出）
├── files/                # 物理存储历史文件和图片（运行自动创建，git 已忽略）
├── config.json           # Token/登录暗号/密保密码（运行自动生成，git 已忽略）
├── .gitignore
└── README.md             # 本说明文档
```

---

## 3. 安全防护与登录体系 (Security & Auth Model)

*   **零入站公网防火墙 (Cloudflare Tunnel)**：VPS 本地服务仅监听 `127.0.0.1:6666`。外部攻击者及全网扫描器无法从公网直接探测该端口。所有请求均由 VPS 主动出站连接 Cloudflare 隧道进行加密中转。
*   **双模式身份鉴权**：
    *   **Token 直接鉴权**：API 请求均需在 HTTP 头部携带 `X-Auth-Token`。图片及二进制文件下载链接支持从 URL 参数中校验 `?token=`。
    *   **Stealth Key 隐形应急门禁**：在新设备/临时环境下，默认访问只显示 Token 鉴权框（完全隐藏密码登录选项）。只有在访问 URL 带有匹配的暗号参数时（如 `?key=vip`），前端才会解锁呈现「使用密保密码登录」的选项。输入 8 位简单密保密码（默认 `66666666`）验证通过后，后端安全分发 Token。
*   **IP 级别指数退避防爆破（Exponential Backoff）**：密码登录接口对每个尝试的 IP 地址进行单独计数。一旦验证失败，该 IP 地址的重试等待惩罚按 $2^{(n-1)}$ 秒呈指数增长（最高延迟 60 秒），彻底瓦解高频暴力攻击。
    *   **CF IP 真实捕获**：后端自动读取 `CF-Connecting-IP` 报头，确保与 Cloudflare 隧道中转时，防爆破锁死能够准确隔离黑客 IP，绝不影响属主用户的 IP 正常访问。
*   **路径防穿透清洗**：所有文件读写动作均执行严格的 `filepath` 安全防线校验，杜绝 `../` 等黑客恶意文件读取注入。

---

## 4. 本地构建与运行 (Local Setup)

### 4.1 编译与启动
在 Go 安装就绪的环境下，直接执行：
```bash
# 启动本地开发服务
go run .
```
首次运行会在项目目录下自动生成 `config.json`，并打印系统自动产生的 32 位安全 Token。默认配置为：
*   `token`：随机产生 32 位密钥
*   `password`：`66666666`（密保密码）
*   `login_key`：`vip`（隐藏 URL 暗号）

### 4.2 访问系统
*   浏览器打开 `http://127.0.0.1:6666`。
*   根据提示输入终端打印的 Token，即可开始使用。
*   应急状态下访问 `http://127.0.0.1:6666/?key=vip`，切换并输入密保密码 `66666666` 登录。

---

## 5. 生产环境编译与 VPS 部署 (Debian/Ubuntu)

### 5.1 交叉编译 (Linux amd64)
在 Windows 开发环境下，执行以下命令编译为 Linux 下的 ELF 二进制可执行文件：
```powershell
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o ics
```

### 5.2 部署目录配置
通过 SCP/SFTP 将编译好的 `ics` 二进制文件与 `static/` 文件夹上传到 VPS 的 `/home/admin/ics/` 目录下。

### 5.3 配置systemd守护进程
创建 `/etc/systemd/system/ics.service`：
```ini
[Unit]
Description=Intranet Chat Stream (ICS) Core Service
After=network.target

[Service]
Type=simple
User=admin
WorkingDirectory=/home/admin/ics
ExecStart=/home/admin/ics/ics
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```
启动并启用自启：
```bash
sudo systemctl daemon-reload
sudo systemctl enable ics --now
```

---

## 6. API 协议规范 (API Contract)

### 6.1 `GET /api/list` (拉取消息流)
*   **请求 Header**：`X-Auth-Token: [您的密钥]`
*   **响应 Header**（附带容量指标）：
    *   `X-Quota-Used`：当前 `./files` 已占用字节数 (bytes)
    *   `X-Quota-Limit`：总磁盘空间配额字节数 (默认 2147483648 bytes, 即 2GB)
*   **响应示例** (按时间戳倒序)：
    ```json
    [
      {
        "id": "1780167900_pc_text.txt",
        "type": "text",
        "content": "验证码: 9981",
        "time": "2026-05-31 03:05:00",
        "pinned": false,
        "device": "pc"
      },
      {
        "id": "1780168200_mobile_screenshot.png",
        "type": "file",
        "filename": "screenshot.png",
        "size": "150 KB",
        "time": "2026-05-31 03:10:00",
        "pinned": true,
        "device": "mobile"
      }
    ]
    ```

### 6.2 `POST /api/push` (投送文本/文件)
*   **请求 Header**：`X-Auth-Token: [您的密钥]`
*   **请求体** (Multipart Form)：
    *   `text` (可选，String)：文本框输入
    *   `file` (可选，File)：上传的文件
    *   `device` (可选，String)：设备标识，支持 `pc`, `mobile`, `ai`, `web`。缺省默认为 `pc`。
*   **响应示例**：
    ```json
    {"id":"1780167900_ai_text.txt","status":"success"}
    ```

### 6.3 `POST /api/login` (密保密码登录)
*   **请求体** (JSON)：
    ```json
    {
      "password": "密保密码",
      "key": "URL参数中的验证暗号Key"
    }
    ```
*   **响应示例 (成功)**：
    ```json
    {
      "status": "success",
      "token": "006e91db7de25746bd029895f9e5f45b"
    }
    ```
*   **响应示例 (等待限速锁定中 - Status 429)**：
    ```json
    {
      "status": "error",
      "error": "尝试次数过多，请等待 12 秒后重试"
    }
    ```

### 6.4 `POST /api/action` (置顶/取消置顶/删除卡片)
*   **请求 Header**：`X-Auth-Token: [您的密钥]`
*   **请求体** (JSON)：
    ```json
    {"id": "1780167900_pc_text.txt", "action": "pin"} 
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

# 1. 拉取当前待处理流并检测容量
res = requests.get(f"{BASE_URL}/api/list", headers=headers)
quota_used = res.headers.get("X-Quota-Used")
quota_limit = res.headers.get("X-Quota-Limit")
print(f"已用容量: {int(quota_used)/1024/1024:.2f} MB / 限额 {int(quota_limit)/1024/1024/1024:.1f} GB")

messages = res.json()
print("当前流中消息数量:", len(messages))

# 2. 推送处理完毕的消息，指定设备标签为 'ai'
payload = {
    "text": "🤖 AI Agent 分析完毕：待提取验证码为 [9981]",
    "device": "ai"
}
res_push = requests.post(f"{BASE_URL}/api/push", headers=headers, data=payload)
print("推送结果:", res_push.json())
```
