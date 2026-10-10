# 云简 Lite

轻量、私有部署的服务器监控平台。**v0.1.0 MVP** 专注于指标采集、实时状态和系统事件，为个人开发者与小型网站站长提供简单的监控工作空间。

> v0.1 不提供远程命令执行、Docker 操作或网站配置管理。后续能力将通过独立 service/API 模块扩展。

## 功能

- 单管理员登录：首次启动生成 256 位随机密码，SQLite 保存 bcrypt 哈希。
- 服务器管理：新增、搜索、状态筛选、改名、删除和 连接密钥 轮换。
- CPU、内存、磁盘使用率与实际容量；网络累计流量和上下行速率。
- 操作系统、系统版本、架构、逻辑核心数与运行时间。
- 监控客户端 每 4 秒上报；90 秒未通信判定离线；后台每 15 秒记录离线事件。
- 前后台 WebSocket 实时通知，断线自动重连，同时每 4 秒轮询兜底。
- 1 / 6 / 24 小时历史趋势；默认保留 7 天指标和事件。
- 连接、离线、CPU/内存/磁盘达到 90% 及恢复事件；支持日志等级和服务器筛选。
- Linux amd64 / arm64 和 Windows amd64 监控客户端；页面提供安装包下载。
- 黑白灰界面，桌面与手机布局，空数据和离线状态提示。

## 界面预览

云简 Lite 提供中文管理控制台和公开监控页面，可在部署后查看实时状态与历史指标。

## 技术架构

后端采用 [Gin](https://gin-gonic.com/en/docs/)、[GORM](https://gorm.io/docs/) 与 SQLite；通过 glebarez/sqlite 适配器使用纯 Go SQLite，发布程序无需 CGO。监控客户端 使用 gopsutil。前端为 Vue 3、Vite、Element Plus，前后端由同一地址提供服务，避免跨域部署复杂度。

```text
云简Lite/
├── server/
│   ├── main.go
│   ├── config/       # 环境配置
│   ├── api/          # 认证、管理控制台和监控上报接口；集成测试
│   ├── models/       # users / sessions / servers / metrics / system_logs
│   ├── database/     # SQLite WAL、迁移、连接管理
│   ├── service/      # 密码初始化、离线检测、数据保留
│   └── websocket/    # 已认证连接与变更通知
├── agent/
│   ├── main.go
│   └── collector/    # CPU / memory / disk / network / system
├── shared/           # 上报协议
├── web/src/
│   ├── views/        # Dashboard / Server / Login / Logs
│   ├── components/   # 指标、趋势、监控客户端安装
│   └── api/          # API 客户端
├── deploy/           # 低资源部署镜像与 systemd 服务模板
├── scripts/          # 构建与 SSH 部署辅助工具
├── docs/             # 安装、开发、协议与验收记录
├── Dockerfile
├── docker-compose.yml
├── go.mod / go.sum
└── LICENSE
```

## Docker 一键安装

需要 Docker Engine 和 Compose v2。推荐 1 GB 以上内存，**从源码构建建议至少 2 GB 内存及 4 GB 可用磁盘**；低资源机器可在其他机器构建后传输发布程序。

```bash
# 将项目放到服务器，进入项目根目录
cp .env.example .env
docker compose up -d --build
docker compose ps
# 查看仅首次生成的密码，不会写入容器日志
docker cp "$(docker compose ps -q server):/app/data/admin-password.txt" ./admin-password.txt
chmod 600 admin-password.txt
cat admin-password.txt
```

默认只绑定 `127.0.0.1:5050`；本机访问 `http://127.0.0.1:5050/admin`，公网请按安装教程配置 HTTPS 和 `PUBLIC_URL`。账号 `admin`。登录后点击右上角账号可以修改密码。修改后首次密码文件会被删除，全部旧会话失效。

SQLite 数据保存在命名卷 `cloudease-lite_cloudease-data`，容器重启与普通 `docker compose down` 不会删除数据。**不要使用 `docker compose down -v`，它会删除数据库。**

公网生产必须使用 HTTPS，设置 `PUBLIC_URL=https://自己的域名`，会自动强制 Secure Cookie。反向代理需转发 WebSocket Upgrade 并保留 Host。客户端默认拒绝向公网 HTTP 上报。示例见 [安装教程](docs/INSTALL.md)。

## 监控客户端安装

1. 登录控制台，点击「添加服务器」，设置名称。
2. 保存首次显示的专属 连接密钥；服务端只保存其 SHA-256 哈希，不能再次查看原值。
3. 在安装弹窗选择 Linux / Windows，复制完整命令到被监控机器执行，再把连接密钥粘贴到隐藏输入提示中；复制命令不包含密钥。Linux 使用 root，自动下载 amd64 / arm64 并配置 systemd；Windows 使用管理员 PowerShell，自动下载 x64 程序并创建开机任务。无需预先上传 监控客户端。

也可以手动下载对应监控客户端，按 [安装教程](docs/INSTALL.md) 创建私有密钥文件后运行：

```bash
chmod +x cloud-agent-linux-amd64
./cloud-agent-linux-amd64 --server https://monitor.example.com --token-file ./agent-token
```

Windows PowerShell：

```powershell
.\cloud-agent.exe --server https://monitor.example.com --token-file ./agent-token
```

每台机器使用不同 连接密钥。一个 连接密钥 对应一条服务器记录，不要在多台机器共享。Linux 支持 Debian、Ubuntu、CentOS；旧 CentOS 的 TLS 根证书应保持可用。amd64/arm64 均提供独立程序。Windows 默认监控系统盘，Linux 默认监控 `/`；可以使用 `--disk` 指定卷。

长期运行请使用 `--token-file` 与 systemd，避免 连接密钥 出现在进程参数中。完整步骤见 [安装教程](docs/INSTALL.md)。`--advertise-ip` 可以覆盖控制台显示的 IP；默认使用上报连接的来源 IP，处于 NAT 后可能为出口 IP。

## 开发环境

- Go **1.26+**（本项目验证使用官方 Go 1.27.1）。
- Node.js 22.12+ 或 24；npm。
- 本地 Go、npm 缓存建议放项目 `.cache/`。仓库可以放在任意磁盘；Go 与 npm 缓存建议放在项目的 `.cache/` 目录，无需全局安装依赖。

```bash
# 在项目根目录
export DATA_DIR="$PWD/.runtime/data"
export GOPATH="$PWD/.cache/gopath"
export GOCACHE="$PWD/.cache/go-build"
go mod download
go run -tags nomsgpack ./server
# 另一个终端
cd web
npm ci --cache ../.cache/npm
npm run dev
```

开发页面为 `http://127.0.0.1:5173`，Vite 自动代理 API 和 WebSocket 到 8080。详见 [开发指南](docs/DEVELOPMENT.md)。

```bash
go test -tags nomsgpack ./...
go vet -tags nomsgpack ./...
cd web && npm run build
```

## 配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LISTEN_ADDR` | `127.0.0.1:8080` | 后端监听地址 |
| `DATA_DIR` | `data` | SQLite 和首次管理员密码目录 |
| `WEB_DIR` | `web/dist` | 前端产物目录 |
| `COOKIE_SECURE` | `false` | HTTPS 部署设为 true |
| `METRIC_RETENTION_DAYS` | `7` | 指标和事件保留天数，1–365 |
| `PORT` | `5050` | Compose 宿主机端口 |
| `BIND_ADDR` | `127.0.0.1` | Compose 宿主机绑定地址 |
| `PUBLIC_URL` | 空 | 公网 HTTPS 来源；配置后固定 Host 和来源 |
| `TRUSTED_PROXY_CIDRS` | 空 | 仅信任明确反代源 CIDR |
| `ALLOW_LEGACY_REPORTS` | `false` | 临时迁移开关，生产应保持 false |
| `DOCKERFILE` | `Dockerfile` | 低资源部署使用 `deploy/Dockerfile.prebuilt` |

## 安全与边界

会话使用 HttpOnly、SameSite=Strict Cookie，默认有效 24 小时。登录限制每 IP 15 分钟最多 10 次失败尝试；跨源写请求和 WebSocket 被拒绝。连接密钥 使用加密随机数，数据库只存哈希。容器使用非 root 用户、只读文件系统和独立持久卷。

CPU/内存/磁盘阈值为固定 90%，仅在跨过阈值时记录事件，不发送邮件或外部告警。网络排除常见虚拟网卡，首个样本速率为 0；重启导致计数减少时速率重置。系统日志是 CloudEase 的事件，不是远程读取 `/var/log`。历史接口最多约 1200 个点，展示完整 24 小时的 4 秒采样数据（按时间桶降采样）。当前不支持多用户、2FA、多个磁盘、流量计费、WebSSH或分布式数据库。

## 开发计划

| 版本 | 计划 |
| --- | --- |
| v0.2 | Docker 容器状态与管理，独立权限控制 |
| v0.3 | 网站配置管理 |
| v0.4 | SSL 证书与过期检测 |
| v0.5 | AI 辅助诊断 |

以上是规划，尚未实现。扩展功能请新增 service/API 模块，不要将任意远程命令混入指标上报协议。

## 贡献指南

提交前运行 Go 测试、vet 与前端构建。安全或持久化改动须有覆盖关键行为的测试。PR 说明问题、改动和验证方法，UI 改动附截图。依赖通过 lockfile/go.sum 固定；不要提交数据库、连接密钥、部署密码、`.env` 或本地缓存。安全问题请先私下联系维护者，避免公开有效凭据。

## 许可证

项目采用 [MIT](LICENSE)。第三方依赖和许可说明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## 前台监控与 4 秒采集
网站首页 / 无需登录即可查看服务器列表；/status/servers/:id 展示资源历史和系统信息。后台入口 /admin，服务器管理和系统日志仍须登录。前台公开名称、系统规格、运行时间、在线状态和资源指标，不公开 IP、连接密钥、账户信息或管理日志。
监控客户端 启动后立即采集，此后每 4 秒采集一次；前后台使用 WebSocket 通知并每 4 秒轮询兜底。历史查询按时间桶取最后一个样本，覆盖完整 1–24 小时窗口，最多约 1200 个点；7 天原始数据仍按原策略保留。已安装的旧 监控客户端 需要升级二进制才能改用 4 秒周期。

## 卸载监控客户端与删除连接密钥

在服务器详情选择「删除连接密钥及服务器」，选择 Linux 或 Windows，复制完整卸载命令到被监控服务器执行。Linux 使用 root，Windows 使用管理员 PowerShell。脚本核对本机密钥后停止客户端、取消开机启动，并仅删除自动安装创建的客户端程序、配置和本机密钥文件；控制台和数据库不会被卸载脚本删除。

然后点击「确认删除密钥及记录」，永久删除控制台中的服务器、指标和日志，连接密钥随即失效。删除记录不会远程执行卸载；命令可在删除记录后继续复制和执行。脚本适用于自动安装方式，手动运行的客户端应先手动停止并清理。若密钥已轮换而客户端尚未更新，校验会拒绝卸载，应先更新客户端配置。

## 安全审计

本地安全加固记录见 [安全审计报告](docs/SECURITY_AUDIT.md)，安全边界与升级要求见 [SECURITY.md](SECURITY.md)。默认上报要求时间戳和随机标识，旧客户端升级步骤见安装教程。
