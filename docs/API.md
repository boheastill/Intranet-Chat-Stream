# ICS API — 外部系统对接契约

> 面向自动化/外部工具对接(如 PC 上的看门狗脚本推送系统通知)。
> 面向 AI 的开发指引见 CLAUDE.md;部署/架构见 README.md 与 ARCHITECTURE_DDD.md。
> 2026-08-15 依据 `bus/` 源码与 `ics-mcp-server/server.py` 实测整理。

## 1. 端点总览

Base URL: `https://flow.bohea.us`(Cloudflare Tunnel → 服务器 `127.0.0.1:6666`)

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/channels` | 列出已有 channel(命名空间),返回 JSON 数组,含 `""` 默认 |
| GET | `/api/list` | 列出消息(可选 `?channel=X`) |
| POST | `/api/push` | 推文本/文件(multipart/form-data,可选 `?channel=X`) |
| POST | `/api/push/chunk/init` | 分片上传:建会话(JSON: filename/size/device) |
| GET | `/api/push/chunk/status?id=` | 分片上传:查已收字节数(断点续传) |
| PUT | `/api/push/chunk/data/{id}?offset=N` | 分片上传:顺序追加原始字节 |
| POST | `/api/push/chunk/complete` | 分片上传:落盘并广播(JSON: id) |
| POST | `/api/action` | pin / unpin / delete(JSON body) |
| GET | `/api/download/{id}` | 取消息/文件原始内容 |
| GET | `/api/stream` | SSE 事件流 |
| POST | `/api/login` | 换 token |

除 `/`、`/index.html`、`/static/*`、`/api/login` 外,所有端点需要鉴权。

**分片上传**:专为"代理按单请求体设限"(如 Cloudflare 免费档 ~100MB 413)的场景——
Web UI 对 >90MB 文件自动切分片(8MB/片),失败自动断点续传;API 调用方同样可用。
另:**登录退避为全局+按 IP 双轨**,错误尝试会让全站错误登录一起变慢
(封顶 60 秒),正确凭证不受影响。

## 2. 鉴权

- Header: `X-Auth-Token: <token>`(也接受 `?token=` 查询参数)
- Token 来源:服务器 `/home/admin/ics/config.json`(首次运行自动生成)
- 本地开发用 token 在 `.mcp.json`(已 gitignore,勿提交)
- 换 token:`POST /api/login`,JSON `{"password":"...","key":"vip"}` → 返回 token;按 IP 指数退避(2^n 秒,上限 60s)

## 3. 推文本消息(最常用)

```bash
curl -X POST \
  -H "X-Auth-Token: <token>" \
  -F "text=<消息内容>" \
  -F "device=pc" \
  "https://flow.bohea.us/api/push?channel=syslog"
```

- 响应:`{"id":"syslog/1786742501_pc_text.txt","status":"success"}`
- `device` 取值惯例:`pc` / `mobile` / `ai`(自定义亦可,会进文件名)
- **channel 机制**:`?channel=X` → 消息存 `files/X/` 子目录,**目录自动创建**;不带 channel 则平铺在 `files/`。通道名会净化(`/`、`\`、`..` 被剔除)
- 消息 ID 规则:`<channel>/<unix时间戳>_<device>_<原始文件名>`

## 4. 推文件

```bash
curl -X POST -H "X-Auth-Token: <token>" \
  -F "file=@/path/to/file.pdf" \
  -F "device=pc" \
  "https://flow.bohea.us/api/push?channel=syslog"
```

## 5. 列出与读取

```bash
curl -H "X-Auth-Token: <token>" "https://flow.bohea.us/api/list"                 # 全部
curl -H "X-Auth-Token: <token>" "https://flow.bohea.us/api/list?channel=syslog"  # 单通道
curl -H "X-Auth-Token: <token>" "https://flow.bohea.us/api/download/syslog/1786742501_pc_text.txt"
```

`/api/list` 返回 JSON 数组,元素形如:
```json
{"id":"syslog/1786742501_pc_text.txt","type":"text","content":"...","time":"2026-08-14 21:21:41","pinned":false,"device":"pc"}
```
文本消息 `content` 内联;文件消息 `content` 为元信息,内容走 `/api/download/{id}`。

## 6. 管理动作(pin / unpin / delete)

```bash
curl -X POST -H "X-Auth-Token: <token>" -H "Content-Type: application/json" \
  -d '{"id":"syslog/1786742501_pc_text.txt","action":"delete"}' \
  "https://flow.bohea.us/api/action"
```

- `action` ∈ `pin` / `unpin` / `delete`;删除带 channel 前缀的 ID 时 **id 必须含 `channel/` 前缀**

## 7. SSE 订阅

```
GET /api/stream?channel=X(可选)
```

事件格式:
```
data: {"event":"new_msg","channel":"<channel>","id":"<id>"}
```

## 8. 现用通道

| channel | 用途 |
|---------|------|
| (无) | 主时间线,人机消息 |
| `syslog` | 系统/网络监控日志(sing-box 看门狗推送,device=pc) |

## 9. 注意

- 存储为扁平文件(`files/`),2GB 滚动配额,删最旧未 pin 消息
- AI pipeline 会消费带触发 token(`@ds`/`@mi`/...)的消息——**纯系统通知类消息不要带这些 token**,避免误触发 AI 回复
- PowerShell 5.1 脚本写消息时保持脚本纯 ASCII(无 BOM 中文会解析失败),消息文本本身可为 UTF-8
