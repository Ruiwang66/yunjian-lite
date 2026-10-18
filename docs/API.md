# v0.3 API 协议（保留 v0.1 / v0.2 接口）

## v0.3 网站接口

以下接口都要求已认证的会话、角色验证和运维审计。`server_id` 指定服务器；站点操作使用 `website_id`，服务端绑定数据库网站和主机上报白名单，客户端再次校验。viewer 可读，POST/PATCH/DELETE 需要 admin/operator。

| 方法 | 路径 | 功能 |
| --- | --- | --- |
| GET / POST | `/api/websites` | 网站列表及创建登记 |
| GET / PATCH / DELETE | `/api/websites/:id` | 网站详情、编辑、删除控制台记录 |
| POST | `/api/websites/:id/check` | 健康、DNS 与 SSL 即时检查 |
| GET | `/api/nginx/status` | 已采集 Nginx 状态 |
| GET | `/api/nginx/sites` | 实时授权站点启用状态（异步） |
| GET | `/api/nginx/site/config` | 授权站点配置读取（异步） |
| POST | `/api/nginx/site/enable`、`disable` | 自动备份、配置检查、重载或失败回滚 |
| GET / POST | `/api/php/info`、`/api/php/inspect` | 缓存 PHP 信息、主动检测 |
| GET / POST | `/api/ssl`、`/api/ssl/check` | 证书记录、主动检测 |
| GET / POST | `/api/domain`、`/api/domain/check` | DNS 记录、主动检测 |
| GET / POST | `/api/backup` | 备份记录、手动执行（`kind=files/database`） |
| GET | `/api/backup/schedule` | 自动备份预留，当前返回 `enabled=false` |

异步站点操作继续返回 202 与任务编号，按 `/api/operations/:task` 读取结果。普通任务 45 秒，备份任务 120 秒到期；不自动重试。创建网站 JSON 为 `name/domain/root_path/server_id/nginx_config`；目录和配置必须与客户端白名单一致，不接受配置文本。客户端认证检查新增 `website_protocol:1`，新管理上报可选 `websites`，旧版协商时不发送此字段。完整边界见 [v0.3 指南](V0.3.md)。

## 保留的基础接口与 v0.2 接口

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


## v0.2 管理接口

地图地区接口：`PATCH /api/servers/:id/location`，请求 `{country_code:"CN"}`；支持地图中的国家或地区代码，空字符串清除地区。需要 admin / operator 登录，沿用来源检查与审计。添加服务器也可选传 `country_code`，公开服务器接口仅增加此地区代码，不公开 IP。

除登录、原有公开监控、healthz 与静态安装脚本外，管理接口全部要求 HttpOnly 会话；客户端管理接口只接受 Bearer 连接密钥，浏览器会话不能替代。所有 API 与 UI 同源，沿用来源验证和 64 KiB 请求上限。

GET 接口需要 `server_id=数字`。POST 接口使用 JSON，公共字段为 `server_id`；未知 JSON 字段拒绝。这里的操作全部针对指定监控服务器，不针对控制台宿主机。

| 方法 | 地址 | 参数 / 功能 | 权限 |
| --- | --- | --- | --- |
| GET | `/api/management/info` | 状态与客户端能力，含允许目录 | 三种角色 |
| GET | `/api/system/info` | 硬件、系统和私有网络信息 | 三种角色 |
| GET | `/api/docker/info` | 最近上报 Docker 状态 / 版本 / 数量 | 三种角色 |
| GET | `/api/docker/container` | 请求最新容器列表 | 三种角色 |
| GET | `/api/docker/container/logs` | `id=容器十六进制编号` | 三种角色 |
| POST | `/api/docker/container/start` | `{server_id,id}` | admin / operator |
| POST | `/api/docker/container/stop` | `{server_id,id}` | admin / operator |
| POST | `/api/docker/container/restart` | `{server_id,id}` | admin / operator |
| GET | `/api/service/list` | systemd 状态与开机启动，最多 200 项 | 三种角色 |
| POST | `/api/service/action` | `{server_id,name,action}`，action 仅 start / stop / restart | admin / operator + 客户端白名单 |
| GET | `/api/files/list` | `root=允许目录索引&path=相对目录` | 三种角色 |
| GET | `/api/files/read` | `root&path`，UTF-8 文本 | 三种角色 |
| GET | `/api/files/download` | `root&path`，文件内容 base64 | 三种角色 |
| POST | `/api/files/upload` | `{server_id,root,path,content}`，content 为 base64 | admin / operator |
| POST | `/api/commands/execute` | `{server_id,name}`，name 仅 disk / memory / network / journal | admin / operator |
| GET | `/api/operations/:task` | 读取自己的任务或管理员读取，编号非顺序 ID | 所有者 / admin |
| GET | `/api/operation-logs` | `page=1&server_id=可选&status=可选`，每页 50 | 三种角色 |
| GET / POST | `/api/users` | 列表 / 新建 `{username,password,role}` | admin |
| PATCH | `/api/users/:id` | `{role,password:可选}`，撤销旧会话 | admin |
| DELETE | `/api/users/:id` | 删除用户，保留审计记录 | admin |
| GET | `/api/roles` | 角色说明 | admin |
| GET | `/api/agent/operations/next` | 领取一次任务；无任务返回 204 | 对应客户端 Bearer 密钥 |
| POST | `/api/agent/operations/result` | `{id,lease,success,data,error}` | 对应密钥 + 任务随机凭证 |

### 异步执行协议

容器、服务、文件和命令操作返回 HTTP 202，例如：

```json
{"id":"43字符的随机操作编号","status":"queued","expires_at":"2026-10-11T10:00:45Z"}
```

随后读取 `/api/operations/:task`，直到 `success`、`failed`、`expired` 或 `cancelled`；最终内容位于 `data`。文件查看 / 下载的 `data` 为 `{content:base64,size,name}`，列表为数组，命令 / 容器日志为 `{output:文本}`。下载 API 返回任务和 base64 内容，由浏览器转换为文件下载，而非直接返回附件。结果保留 5 分钟。

领取的任务包含 `{id,lease,expires_at:Unix毫秒,request:{action,target,root,path,content}}`；客户端用同一个 `lease` 回传。请求不包含 shell 或 argv。服务器再次验证服务器身份、密钥、任务状态、有效期和凭证；每服务器最多 4 条活动任务，同一时间只领取一条。过期的运行任务不自动重发。

### 上报扩展与版本协商

`GET /api/agent/check` 在 v0.2 返回 `management_protocol:1` 与版本。客户端先用自己的 Bearer 密钥协商；旧控制台没有该字段时，上报仍是原 v0.1 结构。v0.2 控制台也接受没有扩展的 v0.1 上报（继续要求原安全版本的 sent_at / nonce）。因此任意一端可先升级；早于时间戳安全升级的客户端仍需升级或短期打开 ALLOW_LEGACY_REPORTS。

新 `Report` 的可选字段：

- `system`：cpu_model、kernel_version、virtualization、private_ips、public_ips。
- `management`：version、enabled、privileged、docker_write、service_write、file_write、file_roots、docker、services。

重型 Docker / systemd 状态每 30 秒后台采集，CPU / 内存 / 磁盘 / 网络仍每 4 秒上报。增强信息仅通过认证接口展示，原有公开投影不会包含 IP、目录、角色或运维结果。
