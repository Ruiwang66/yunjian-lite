# 开发说明

## Windows（使用项目内缓存）

Go 可解压官方 Windows ZIP 到 `.tools/go/`，无需安装或修改系统 PATH。

```powershell
# 在项目目录打开 PowerShell
$env:GOPATH = "$PWD\.cache\gopath"
$env:GOCACHE = "$PWD\.cache\go-build"
$env:GOTMPDIR = "$PWD\.cache\tmp"
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:npm_config_cache = "$PWD\.cache\npm"
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
$env:DATA_DIR = "$PWD\.runtime\data"
$env:WEB_DIR = "$PWD\web\dist"
& .\.tools\go\bin\go.exe run -tags nomsgpack ./server
# 另一个终端
npm.cmd ci --prefix web
npm.cmd run dev --prefix web
```

Vite 监听 127.0.0.1:5173，并代理 API/WS、安装/卸载脚本到 8080。开发时后台初次认证检查可能收到预期的 401；登录后正常。

## 测试与构建

```powershell
& .\.tools\go\bin\go.exe test -tags nomsgpack ./...
& .\.tools\go\bin\go.exe vet -tags nomsgpack ./...
.\scripts\build.ps1
```

`build.ps1` 输出：

```text
bin/cloud-server                 Linux amd64 后端
bin/cloud-server.exe             Windows amd64 后端（本地验证）
bin/downloads/cloud-agent-linux-amd64
bin/downloads/cloud-agent-linux-arm64
bin/downloads/cloud-agent.exe
web/dist/                        前端生产资源
```

后端 Linux 发布程序无 CGO，不依赖宿主机 glibc。源码 Dockerfile 的后端架构随构建宿主机；预编译 Dockerfile 当前要求 Linux amd64 主机。

## Linux / macOS

```bash
mkdir -p .cache/tmp
export GOPATH="$PWD/.cache/gopath"
export GOCACHE="$PWD/.cache/go-build"
export GOTMPDIR="$PWD/.cache/tmp"
go test -tags nomsgpack ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags nomsgpack -trimpath -ldflags='-s -w' -o bin/cloud-server ./server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/downloads/cloud-agent-linux-amd64 ./agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o bin/downloads/cloud-agent-linux-arm64 ./agent
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o bin/downloads/cloud-agent.exe ./agent
npm ci --prefix web --cache .cache/npm
npm run build --prefix web
```

## 数据和模块边界

- `shared.Report` 为唯一的 监控客户端 上报协议。
- `models` 是 GORM schema；启动时 AutoMigrate 仅创建或增加结构，不清空数据库。
- SQLite WAL + 单数据库连接，避免小型部署竞争写入；报告及关联日志同一事务入库。
- 服务器最新指标按 server_id + created_at 索引读取；历史接口限制到 24 小时与约 1200 个时间桶。
- 每小时删除过期指标、日志和会话；SQLite 会重用释放页，不保证文件立即收缩。
- 连接密钥 为 32 随机字节的 base64url 编码，DB 存 SHA-256。管理员密码为 bcrypt。
- WebSocket 只传通知，前端通过已经认证的 HTTP 接口取最新数据。
- v0.2 管理业务通过独立 management 模块和异步任务；仅客户端明确开启时执行固定动作。未来网站/SSL 业务沿用白名单，禁止任意 shell。

## 当前验收覆盖

集成测试通过真实 httptest/Gin/GORM/SQLite 检查认证、来源保护、连接密钥 哈希、首次连接、告警去重、频率限制、离线状态、重连、连接密钥 轮换、非法指标、WebSocket 通知与跨源拒绝、密码修改和会话失效、级联清理。

实际运行另外验证 Windows 主机采集与 Linux VPS 监控客户端、浏览器 UI、容器健康检查和重启持久化。详见 `VERIFICATION.md`。Windows/Ubuntu/CentOS 全版本兼容性仍应通过维护者的目标环境验证。

## 安全回归验证（仅本地）

```powershell
node tests/client-install-commands.cjs
node tests/client-removal-commands.cjs
.\tests\client-install-windows.ps1
.\tests\client-uninstall-windows.ps1
# 使用项目内 Python 执行；只启动隔离回环进程，不安装服务
python tests/security-smoke.py
```

安装/卸载 Windows fixture 的网络和任务 API 使用 mock，仅文件操作发生在 .runtime 内；不能代表真实开机任务安装通过。Linux 卸载 fixture 必须在一次性 Docker 容器中运行，脚本会拒绝原生宿主执行。Windows 的 NTFS 私有 ACL 由 Go 回归测试实际检查；Linux 权限测试应在 Linux 测试环境运行。

依赖复查可使用 govulncheck -json，必须解析 findings；JSON 模式返回 0 不等于没有模块告警。Npm 审计使用官方 registry。Docker 构建与目标系统运行应单独验收。
