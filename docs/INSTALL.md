# 安装与运维

## 控制台

项目根目录运行：

```bash
cp .env.example .env
docker compose up -d --build
# 等待 healthy
docker compose ps
docker cp "$(docker compose ps -q server):/app/data/admin-password.txt" ./admin-password.txt
chmod 600 admin-password.txt
cat admin-password.txt
```

默认后台绑定 `127.0.0.1:5050`，仅本机可访问，账号 `admin`。首次密码只生成一次，不会在重启时覆盖。公网访问必须配置下方 HTTPS 反代和 `PUBLIC_URL`，只对公网开放 443，保留 5050 回环监听。不要向外网开放数据库卷或 `.env`。登录后立即修改首次密码。

低内存主机：先在开发机器运行 `scripts/build.ps1`（Windows）或按 `docs/DEVELOPMENT.md` 构建，然后上传 `bin/cloud-server`、`bin/downloads/` 与 `web/dist/`，设置 `.env`：

```dotenv
DOCKERFILE=deploy/Dockerfile.prebuilt
```

此时仍使用 `docker compose up -d --build`；主机无需 Go/Node，只构建小型运行镜像。

## 自动安装 监控客户端（推荐）

在后台创建服务器后，选择 Linux 或 Windows，通过 HTTPS 访问控制台，复制完整安装命令到被监控服务器执行，再复制密钥并粘贴到隐藏输入提示，无需预先下载文件。复制命令不再包含密钥；无人值守安装可使用权限受限的 --key-file / -KeyFile。安装脚本使用该服务器的 连接密钥 下载二进制，不依赖浏览器登录 Cookie；脚本地址不带 连接密钥，连接密钥 仅通过 Authorization 请求头校验。

Linux 使用 root 执行，要求 systemd 和 curl 或 wget，自动识别 amd64 / arm64。脚本先校验连接密钥、下载程序并核对 SHA-256 和可执行文件头，再安装到 `/opt/cloudease-lite/cloud-agent`，保存私有 连接密钥 文件，启用 `cloud-agent` systemd 服务。重复执行会更新同一 监控客户端 服务。

Windows 使用管理员 PowerShell 执行，支持 x64。安装目录为 `%ProgramData%\CloudEaseLite`，连接密钥仅 SYSTEM、管理员和 LocalService 可读；创建以 LocalService 低权限运行的 `CloudEaseLite-Agent` 开机任务并立即启动。安装本身需要管理员权限。

已有 连接密钥 仍可使用，无需为下载重新创建服务器。手动下载与运行方式见下文。脚本不会删除服务器记录或数据库。

## Linux 监控客户端（Debian / Ubuntu / CentOS）

在控制台创建服务器并下载对应架构的 监控客户端。通过 SSH/scp 上传文件。以下命令需 root；**配置中的控制台地址必须替换为自己的地址**。

```bash
id cloudease-agent >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin cloudease-agent
install -d -m 0755 /opt/cloudease-lite
install -d -m 0750 -o root -g cloudease-agent /etc/cloudease-lite
install -m 0755 cloud-agent-linux-amd64 /opt/cloudease-lite/cloud-agent
# 创建连接密钥文件：粘贴自己的连接密钥，然后 Ctrl+D；输入不会进入 shell history
install -m 0640 -o root -g cloudease-agent /dev/null /etc/cloudease-lite/agent-token
cat > /etc/cloudease-lite/agent-token
# 按需替换两项配置。空 CLOUDEASE_IP 表示显示上报来源 IP。
printf 'CLOUDEASE_SERVER=https://monitor.example.com\nCLOUDEASE_IP=\n' > /etc/cloudease-lite/agent.env
chmod 0640 /etc/cloudease-lite/agent.env
chown root:cloudease-agent /etc/cloudease-lite/agent.env
cp deploy/cloud-agent.service /etc/systemd/system/cloud-agent.service
systemctl daemon-reload
systemctl enable --now cloud-agent
systemctl status cloud-agent --no-pager
journalctl -u cloud-agent -n 30 --no-pager
```

同台主机部署 Server 时，监控客户端 地址可以为 `http://127.0.0.1:5050`，`CLOUDEASE_IP` 设置公网 IP，监控数据通过 loopback 上报。

CentOS 如无 `/usr/sbin/nologin`，使用 `/sbin/nologin`。如启用 SELinux，须按实际策略允许执行安装路径；不要为此关闭 SELinux。二进制为静态 Go 程序，无需 glibc。

## Windows 监控客户端

在控制台下载 `cloud-agent.exe`。使用 PowerShell：

推荐使用自动安装弹窗，它会在写入前设置私有 ACL。手动方式需先创建空的 `agent-token` 文件，移除继承，仅授予当前账户、SYSTEM 和管理员读取权限，再通过隐藏输入写入密钥；不要先向权限开放的文件写入密钥。然后运行：

```powershell
.\cloud-agent.exe --server https://monitor.example.com --token-file .\agent-token
```

可以通过 Windows 任务计划程序设置开机启动，将「起始于」设为 监控客户端 所在目录。默认采集系统盘；`--disk D:\` 可指定其他盘。不要将 Windows 程序作为 Linux 安装包使用。

## 公网 HTTPS 反向代理

以下片段加入已有 Nginx 的独立 server 块，替换域名与证书路径。**该配置需要有效域名和证书，不会自动申请证书。**

```nginx
server {
    listen 443 ssl;
    server_name monitor.example.com;
    ssl_certificate /path/fullchain.pem;
    ssl_certificate_key /path/privkey.pem;
    location / {
        proxy_pass http://127.0.0.1:5050;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 120s;
    }
}
```

Compose `.env` 配置：

```dotenv
BIND_ADDR=127.0.0.1
PUBLIC_URL=https://monitor.example.com
COOKIE_SECURE=true
ALLOW_LEGACY_REPORTS=false
TRUSTED_PROXY_CIDRS=
```

HTTPS PUBLIC_URL 会自动强制 Secure Cookie，并固定允许的浏览器来源与 Host；后端不依赖任意传入的 X-Forwarded-Proto。代理须转发公网 Host；非标准端口时保留端口，例如使用经过校验的 $http_host。首次密码可通过 SSH 隧道访问回环测试，正式管理入口用 HTTPS。

默认不信任 X-Forwarded-For。需要真实客户端 IP 时，先确认后端看到的代理源 IP，再把它以明确的 /32 或 /128 CIDR 写入 TRUSTED_PROXY_CIDRS；容器里可能是 Docker 网桥网关，不应盲目写主机 127.0.0.1。代理必须覆盖而非透传不可信的头，不要配置 0.0.0.0/0，程序会拒绝 /0 全地址信任。未配置时限额会由经过同一代理的用户共享；也可使用客户端 --advertise-ip 显式显示 IP。

## 密码重置

显式重置会立即撤销全部会话：

```bash
docker compose exec server /app/cloud-server --reset-admin-password
docker cp "$(docker compose ps -q server):/app/data/admin-password.txt" ./admin-password.txt
chmod 600 admin-password.txt
cat admin-password.txt
```

这是本地管理命令，不是公开接口。重置不删除指标和服务器。

## 备份与恢复

先暂停写入再归档，避免 SQLite WAL 不完整：

```bash
umask 077
mkdir -p backups
docker compose stop server
# 运行服务的一次性容器，无网络端口、无新数据库写入
backup_dir="backups/cloudease-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup_dir"
docker cp "$(docker compose ps -a -q server):/app/data/." "$backup_dir/"
tar -C "$backup_dir" -czf "$backup_dir.tar.gz" .
docker compose start server
chmod 600 backups/*.tar.gz
```

恢复时先停止服务、确认备份内容，在离线数据库卷中恢复整组文件（`.db`、`-wal`、`-shm` 如存在）。不要将旧数据库直接覆盖在运行中的服务上。建议恢复到新卷并验证后再切换；备份包含 连接密钥 哈希、会话和首次密码文件，需保密。

## 更新与停止

```bash
# 先备份，再更新源码/发布程序
docker compose up -d --build
# 重启、日志、停止
docker compose restart server
docker compose logs --tail=100 server
docker compose down
systemctl stop cloud-agent
```

`docker compose down` 保留命名卷；加 `-v` 将永久删除数据库。监控客户端 停止后 90 秒被判定离线，后台最多再延迟 15 秒记录离线日志。

## 排查

- 登录失败：核对密码文件，或用显式重置命令；10 次失败后当前来源 IP 需等待 15 分钟。
- 连接密钥 无效：检查文件空白和控制台记录；重新生成 连接密钥 后必须更新 监控客户端 文件并重启。
- 5050 不通：默认只允许本机连接；公网应检查 HTTPS 443 反代、Compose 健康状态与实际绑定地址，不应为排障直接开放明文管理端口。
- WebSocket 断开：检查代理 Upgrade、Host、超时；界面仍会每 4 秒尝试获取数据。
- SQLite 权限错误：容器 UID/GID 为 10001，命名卷会自动初始化权限；自定义 bind mount 需要自行赋权。
- 网络首条速率 0：正常，需要两个采样点计算变化率。

## 卸载监控客户端与删除连接密钥

在服务器详情选择「删除连接密钥及服务器」，选择 Linux 或 Windows，复制完整卸载命令到被监控服务器执行。Linux 使用 root，Windows 使用管理员 PowerShell。脚本核对本机密钥后停止客户端、取消开机启动，并仅删除自动安装创建的客户端程序、配置和本机密钥文件；控制台和数据库不会被卸载脚本删除。

然后点击「确认删除密钥及记录」，永久删除控制台中的服务器、指标和日志，连接密钥随即失效。删除记录不会远程执行卸载；命令可在删除记录后继续复制和执行。脚本适用于自动安装方式，手动运行的客户端应先手动停止并清理。若密钥已轮换而客户端尚未更新，校验会拒绝卸载，应先更新客户端配置。

## 本地安全版本升级要求

本次修改仅在本地完成，不代表任何已部署服务器已经升级。升级前备份数据与旧发布程序。推荐先升级监控客户端，再升级控制台：旧控制台会忽略新字段，新控制台默认要求时间戳和随机标识。

客户端每 4 秒上报，控制台允许五分钟时钟偏差，并拒绝倒序和重复上报；所有机器应同步时钟，一条记录不要运行多个客户端。若无法同时升级，可短期使用 ALLOW_LEGACY_REPORTS=true，完成升级后立即改回 false。旧客户端缺失字段时会被默认拒绝，不会删除历史数据。

API 限额为每 IP 5 次/秒、突发 60；WebSocket 总量 256、每 IP 8。大量同 NAT 的客户端与浏览器会共享 IP 限额，应按规模审慎调整源码参数并配合反代限流。不要关闭校验来适配不可信代理。

自动安装新增下载校验接口，需要控制台也提供 /api/agent/downloads/:file/sha256。安装脚本和二进制必须来自同一版本。Windows 自动安装服务账户改为 LocalService，应在目标 Windows Server 上验证开机启动与需要监控的卷是否可读。

Go/Node 构建标签固定到验证版本，但仍不是不可变 digest。正式发布前验证镜像来源并固定 digest、生成发布清单和独立签名。项目本地审计不代表目标 Docker、TLS、防火墙和开机任务已实测。
