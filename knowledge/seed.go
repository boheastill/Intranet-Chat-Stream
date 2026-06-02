package knowledge

import "ics/ai"

// projectKnowledge is built-in, version-controlled knowledge about the ICS
// project itself. It is upserted into the store on every startup (see
// Store.seedProject), so the code stays the single source of truth while any
// other entries the store has accumulated are preserved.
//
// SECURITY: AI replies are visible to anyone who can trigger the pipeline, so
// these entries describe mechanisms and operations but deliberately contain NO
// live secret values (passwords, login key, auth token, API keys). Do not add
// real credentials here.
var projectKnowledge = []ai.Entry{
	{
		Topic:   "ICS 是什么",
		Content: "ICS（Intranet Chat Stream，内网聊天流）是一个跨设备消息总线，内置 AI 管道。任何设备通过网页或 API 发消息即可跨设备实时同步，并能用触发词唤起 AI 回复。公开地址 https://flow.bohea.us。",
		Tags:    []string{"是什么", "介绍", "项目", "功能", "干什么", "ics"},
	},
	{
		Topic:   "如何触发 AI",
		Content: "在消息里用触发词唤醒 AI：@ds 用 DeepSeek，@mi 用小米 MiMo，@ag 为预留(暂走 DeepSeek)，@cc 是旧触发词(现等价 MiMo)。触发词后面写上你的问题即可；只发触发词不带内容会返回用法提示。",
		Tags:    []string{"触发", "怎么用", "命令", "唤醒", "@ds", "@mi", "@cc", "@ag", "模型"},
	},
	{
		Topic:   "AI 大脑是谁",
		Content: "ICS 的 AI 回复由两个后端提供：DeepSeek(deepseek-chat) 与小米 MiMo(mimo-v2.5-pro)，经 Go 编写的 pipeline 按需调用，无状态(Mode C)。并非 GPT、Claude 或其它模型。",
		Tags:    []string{"你是谁", "哪个模型", "什么模型", "大脑", "deepseek", "mimo", "后端", "model"},
	},
	{
		Topic:   "整体架构",
		Content: "ICS 是单个 Go 二进制(module/二进制/service 均名为 ics)，内含两个核心包：bus(消息总线，REST + SSE，文件存储无数据库) 和 pipeline(监听消息→按触发词路由→调用 AI 后端)。前端是单页 Web(static/index.html)。",
		Tags:    []string{"架构", "设计", "原理", "结构", "怎么实现", "go", "bus", "pipeline"},
	},
	{
		Topic:   "消息与文件存储",
		Content: "所有消息和文件以扁平文件存放于 ./files，无数据库；文件名编码元数据(时间戳_设备_原名)。新消息在最上方，支持置顶保护，总量超过 2GB 配额时自动删除最旧的未置顶文件。",
		Tags:    []string{"存储", "文件", "消息", "置顶", "容量", "配额", "删除"},
	},
	{
		Topic:   "部署与运维",
		Content: "用仓库根的 deploy.ps1 从 Windows 交叉编译 Linux 二进制，scp 到 AWS 新加坡(47.130.65.208)，以 systemd service `ics` 运行，工作目录 /home/admin/ics。更新流程：停服→上传→启动→看日志。",
		Tags:    []string{"部署", "deploy", "服务器", "运维", "aws", "systemd", "发版"},
	},
	{
		Topic:   "网络与访问",
		Content: "服务只监听 127.0.0.1:6666，不开公网入站端口；由 Cloudflare Tunnel 主动出站把 https://flow.bohea.us 的流量转发到本地。这样全网扫描器无法直接探测服务端口。",
		Tags:    []string{"网络", "访问", "端口", "cloudflare", "tunnel", "域名", "公网"},
	},
	{
		Topic:   "认证机制",
		Content: "所有 API(除静态页与登录)需携带 X-Auth-Token(请求头或 ?token= 参数)。网页可用密码+URL 暗号登录换取 token，登录失败按 IP 指数退避防爆破。(出于安全，此处不公开具体密码、暗号与 token 值。)",
		Tags:    []string{"认证", "鉴权", "登录", "token", "密码", "安全", "auth"},
	},
	{
		Topic:   "配置与环境变量",
		Content: "首次运行自动生成 config.json(token/密码/暗号)。AI 密钥不硬编码，经 systemd EnvironmentFile(/home/admin/ics/ics.env) 注入 DEEPSEEK_API_KEY、MIMO_API_KEY；另有 ICS_MAX_DIR_SIZE_BYTES、MIMO_MODEL 等可覆盖。",
		Tags:    []string{"配置", "环境变量", "env", "config", "密钥", "apikey"},
	},
	{
		Topic:   "MCP 工具",
		Content: "ics-mcp-server(Python/FastMCP) 把 ICS 的 REST API 封装成 4 个 MCP 工具：list_messages、read_message、push_message、manage_message，让外部 AI 客户端能直接读写消息流。",
		Tags:    []string{"mcp", "工具", "集成", "server", "python"},
	},
	{
		Topic:   "设计哲学",
		Content: "核心理念：Dumb Pipe(总线只搬消息不思考)、Mode C(无状态 AI，知识外置按需调用)、简单优先(文件胜过数据库)、异构容错。架构复杂度匹配场景(用户量=1)。",
		Tags:    []string{"哲学", "理念", "原则", "设计思想", "为什么"},
	},
	{
		Topic:   "作者与用途",
		Content: "ICS 是 bohea 的个人项目，用于在自己的多台设备之间稳定、干净地流转消息与文件，并让 AI 跨设备协助。",
		Tags:    []string{"作者", "谁做的", "用途", "bohea", "为什么做"},
	},
}
