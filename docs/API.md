# v0.1 API 协议

所有 JSON API 位于 `/api`；错误为 `{ "error": "..." }`。标准状态码 400/401/403/404/409/413/415/421/429/500。请求体最多 64 KiB；JSON 必须使用 application/json，拒绝未知字段和尾随值。没有 CORS 开放接口。通过同源 Cookie 登录控制台；监控客户端 通过独立 Bearer 连接密钥 上报。

| 方法 | 路径 | 认证 | 功能 |
| --- | --- | --- | --- |
| GET | /healthz | 无 | SQLite 可用性和版本 |
| POST | /api/auth/login | 无 | username/password，返回 user 并设置 Cookie |
| GET | /api/auth/me | 会话 | 当前用户，不包含密码 |
| POST | /api/auth/logout | 会话 | 删除当前会话和 Cookie |
| POST | /api/auth/password | 会话 | current_password/new_password；撤销全部会话 |
| GET | /api/servers | 会话 | 服务器列表 + 最新样本 |
| POST | /api/servers | 会话 | name；返回 server/token，连接密钥 仅返回一次 |
| GET | /api/servers/:id | 会话 | 详情 |
| PATCH | /api/servers/:id | 会话 | name |
| GET | /api/servers/:id/uninstall | 会话 | 卸载校验值，不返回原密钥，no-store |
| DELETE | /api/servers/:id | 会话 | 清理服务器、指标和事件 |
| POST | /api/servers/:id/token | 会话 | 轮换 连接密钥，旧 连接密钥 即失效 |
| GET | /api/servers/:id/metrics?hours=6 | 会话 | 1–24 小时历史，按时间升序，最多约 1200 个点 |
| GET | /api/logs | 会话 | level/server_id/page/limit，最多每页 100 条 |
| GET | /api/ws | 会话 | WebSocket，消息 type/at |
| GET | /api/downloads/:file/sha256 | 会话 | 同一下载白名单的 SHA-256，no-store |
| GET | /api/downloads/:file | 会话 | 固定白名单 监控客户端 文件 |
| GET | /install-client.sh | 无 | Linux 自动安装脚本，不包含 连接密钥 |
| GET | /install-client.ps1 | 无 | Windows x64 自动安装脚本，不包含 连接密钥 |
| GET | /api/agent/check | 监控客户端 | 仅校验 Bearer 连接密钥，不修改监控数据 |
| GET | /api/agent/downloads/:file/sha256 | 监控客户端 | SHA-256 校验值，仅 Bearer 认证 |
| GET | /api/agent/downloads/:file | 监控客户端 | 相同固定白名单，供安装脚本下载 |
| POST | /api/agent/report | 监控客户端 | 监控采样，每服务器最短间隔 3 秒 |

## 监控客户端 报告

Header：`Authorization: Bearer TOKEN` 与 `Content-Type: application/json`。

```json
{
  "sent_at": 1791417600000,
  "nonce": "AAAAAAAAAAAAAAAAAAAAAA",
  "name": "debian-host",
  "ip": "192.0.2.10",
  "os": "linux",
  "platform": "debian",
  "platform_version": "12",
  "arch": "x86_64",
  "cpu_cores": 2,
  "cpu": 12.4,
  "memory": 40.0,
  "memory_total": 2147483648,
  "memory_used": 858993459,
  "disk": 50.0,
  "disk_total": 42949672960,
  "disk_used": 21474836480,
  "network_rx": 1000000,
  "network_tx": 300000,
  "network_rx_rate": 1200,
  "network_tx_rate": 600,
  "uptime": 3600
}
```

百分比范围 0–100，容量和累计计数为字节，速率为字节/秒，uptime 为秒。name 不覆盖管理员设置的名称，ip 为空时使用连接来源 IP。整数拒绝负数和超过有符号 64 位范围的值，used 不能大于 total。sent_at 为当前 Unix 毫秒，nonce 是 16 字节随机数的无填充 base64url 编码；示例值只说明格式，必须逐次生成。默认缺失字段返回 400；时钟偏差超过五分钟返回 400；已提交或倒序时间戳、重复随机标识返回 409，不写入数据。

返回 `{ "ok": true, "server_id": 1, "next_report_seconds": 4 }`。监控客户端 首次立即采集，此后固定 4 秒周期。采集失败或报告失败记入 监控客户端 本地日志；下一个周期重试，不执行无限高频重试。

## WebSocket

握手后首先发送 `{"type":"connected"}`。后续消息为 `metrics_updated`、`servers_changed` 或 `status_changed`，带 UTC 时间 at。它们只提示重新取数据，不公开完整指标。连接每 25 秒 ping，客户端 70 秒内无响应则断开。会话到期时关闭连接，撤销后最长 25 秒关闭连接，HTTP 接口立即拒绝撤销会话。

## 日志和时间

指标、事件、last_seen、会话过期以及 SQLite 时间查询边界使用 UTC，避免 SQLite 对时区字符串进行错误比较。浏览器统一按本地时区展示。告警在 90% 阈值跨越时写入，离线后重连写 INFO，不每次上报重复写告警。

## 公开监控接口
以下接口无需登录，仅开放只读监控数据：
- GET /api/public/servers：服务器公开列表，不含 IP 或 连接密钥。
- GET /api/public/servers/:id：公开服务器详情。
- GET /api/public/servers/:id/metrics?hours=6：1–24 小时历史，最多约 1200 个时间桶，每桶取最后一个采样。
- GET /api/public/ws：同源 WebSocket，消息仅含通知类型与时间，连接最长 24 小时后自动重连。
公开监控接口没有服务器增删、连接密钥、下载或日志权限，相关管理接口仍使用原登录认证。监控客户端 安装接口另行使用 Bearer 连接密钥，仅开放校验与固定白名单下载；不接受 URL 查询参数中的 连接密钥。

## 资源与来源限制

所有 API no-store；每 IP 5 次/秒、突发 60，请求键最多 10000。WebSocket 全局 256、每 IP 8、读消息 1 KiB，禁止客户端发送应用消息（关闭码 1008）。客户端密钥不能授权后台，管理 Cookie 不能授权上报。登录最多并发 4 次 bcrypt，原每 IP 失败次数限制仍保留。

PUBLIC_URL 固定管理页面和接口 Host、写请求 Origin 和 WebSocket Origin。HTTPS 自动启用 Secure Cookie 和 HSTS；默认不信任代理头，仅显式 TRUSTED_PROXY_CIDRS 影响 ClientIP。上报和健康检查允许从回环入口访问。
