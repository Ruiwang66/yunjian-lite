#!/usr/bin/env bash
# CloudEase Lite: install a Linux Agent and enable it at boot.
set -euo pipefail
umask 077
die() { echo "安装失败：$*" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || die "请在被监控服务器上使用 root 执行。"
[ "$(uname -s)" = Linux ] || die "此脚本仅支持 Linux。"
command -v systemctl >/dev/null && [ -d /run/systemd/system ] || die "需要正在运行的 systemd。"
server= token= key_file=
management=false
container_monitor=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        --management) management=true; shift ;;
        --container-monitor) container_monitor=true; shift ;;
        --server) [ "$#" -ge 2 ] || die "--server 缺少地址"; server="$2"; shift 2 ;;
        --key-file) [ "$#" -ge 2 ] || die "缺少密钥文件"; key_file="$2"; shift 2 ;;
        --key|--token) [ "$#" -ge 2 ] || die "缺少连接密钥"; token="$2"; shift 2 ;;
        *) die "未知参数：$1" ;;
    esac
done
if [ "$container_monitor" = true ]; then
    [ "$management" = false ] || die "--container-monitor 仅支持监控，不能同时启用 --management。"
    command -v systemd-detect-virt >/dev/null && systemd-detect-virt --container --quiet || die "容器兼容模式仅用于 systemd 能识别的容器。"
    # A missing service is expected on first installation; other inspection errors are not.
    unit_info=$(systemctl show cloud-agent.service -p LoadState -p DropInPaths 2>/dev/null) || {
        [[ "$unit_info" == *'LoadState=not-found'* ]] || die "无法检查现有服务覆盖配置，原有安装未修改。"
    }
    drop_ins=$(printf '%s\n' "$unit_info" | sed -n 's/^DropInPaths=//p')
    [ -z "$drop_ins" ] || die "存在 systemd drop-in：$drop_ins。请先检查并备份冲突配置；脚本不会自动删除覆盖配置。"
    for dir in /etc/systemd/system/cloud-agent.service.d /run/systemd/system/cloud-agent.service.d /usr/lib/systemd/system/cloud-agent.service.d /lib/systemd/system/cloud-agent.service.d; do
        for file in "$dir"/*.conf; do
            [ ! -e "$file" ] || die "存在服务覆盖配置：$file。请先检查并备份冲突配置，原有安装未修改。"
        done
    done
    echo "明确选择容器仅监控：保留普通账户、空能力集和 128 MB 服务内存限制，停用客户端服务中不适用的隔离项。外层容器权限不变。"
fi
server_pattern='^https?://([a-zA-Z0-9.-]+|\[[0-9a-fA-F:]+\])(:[0-9]+)?/?$'
[[ "$server" =~ $server_pattern ]] || die "控制台地址须为 http(s)://域名或IP，可包含端口。"
server="${server%/}"
if [[ "$server" == http://* ]] && [[ ! "$server" =~ ^http://(localhost|127\.0\.0\.1|\[::1\])(:[0-9]+)?$ ]]; then
    die "公网安装必须使用 HTTPS，HTTP 仅允许本机回环地址。"
fi
if [ -n "$key_file" ]; then
    [ -f "$key_file" ] && [ ! -L "$key_file" ] || die "密钥须为普通文件。"
    [ "$(stat -c %s "$key_file")" -le 512 ] || die "密钥文件过大。"
    mode=$(stat -c %a "$key_file")
    [ "$((8#$mode & 0027))" -eq 0 ] || die "密钥文件须拒绝其他用户访问及组写入。"
    token=$(cat -- "$key_file")
fi
if [ -z "$token" ]; then
    IFS= read -rsp "输入连接密钥（不显示）：" token || die "未读取到连接密钥。"
    printf '\n'
fi
[[ "$token" =~ ^[A-Za-z0-9_-]{43}$ ]] || die "连接密钥格式无效，请完整复制控制台生成的安装命令。"
case "$(uname -m)" in
    x86_64|amd64) binary=cloud-agent-linux-amd64 ;;
    aarch64|arm64) binary=cloud-agent-linux-arm64 ;;
    *) die "暂不支持此 CPU 架构：$(uname -m)" ;;
esac
work=$(mktemp -d /tmp/cloudease-client.XXXXXXXX)
trap 'rm -rf -- "$work"' EXIT
printf 'header = "Authorization: Bearer %s"\n' "$token" > "$work/curl.conf"
printf 'header = Authorization: Bearer %s\n' "$token" > "$work/wget.conf"
fetch() {
    local url="$1" output="$2" status
    if command -v curl >/dev/null; then
        status=$(curl --config "$work/curl.conf" --fail --silent --show-error --connect-timeout 15 --max-time 120 -o "$output" -w '%{http_code}' "$url") || return 1
        [ "$status" = 200 ] || { echo "下载返回 HTTP $status，请检查控制台地址（不跟随重定向）。" >&2; return 1; }
    elif command -v wget >/dev/null; then
        wget --config="$work/wget.conf" --quiet --timeout=30 --tries=2 --max-redirect=0 -O "$output" "$url"
    else
        die "请先安装 curl 或 wget。"
    fi
}
echo "校验连接密钥并下载对应架构的监控客户端..."
fetch "$server/api/agent/check" "$work/check.json" || die "控制台不可达或 连接密钥无效；原有安装未修改。"
fetch "$server/api/agent/downloads/$binary" "$work/cloud-agent" || die "监控客户端下载失败；原有安装未修改。"
[ -s "$work/cloud-agent" ] || die "下载文件为空。"
fetch "$server/api/agent/downloads/$binary/sha256" "$work/checksum" || die "无法读取安装包校验值。"
expected=$(tr -d '\r\n' < "$work/checksum")
[[ "$expected" =~ ^[a-f0-9]{64}$ ]] || die "安装包校验值格式无效。"
actual=$(sha256sum "$work/cloud-agent"); actual="${actual%% *}"
[ "$actual" = "$expected" ] || die "安装包校验失败，未执行下载的程序。"
magic=$(od -An -tx1 -N4 "$work/cloud-agent" | tr -d ' \n')
[ "$magic" = 7f454c46 ] || die "安装包不是 Linux 程序。"
chmod 0755 "$work/cloud-agent"
for dir in /opt/cloudease-lite /etc/cloudease-lite; do
    [ ! -L "$dir" ] || die "安装目录是符号链接，已停止。"
    if [ -d "$dir" ]; then
        [ "$(stat -c %u "$dir")" -eq 0 ] || die "已有安装目录不是 root 所有。"
        mode=$(stat -c %a "$dir")
        [ "$((8#$mode & 0022))" -eq 0 ] || die "已有安装目录允许其他账户写入。"
    fi
done
for file in /opt/cloudease-lite/cloud-agent /etc/cloudease-lite/agent-token /etc/cloudease-lite/agent.env /etc/systemd/system/cloud-agent.service; do
    [ ! -L "$file" ] || die "已有客户端文件是符号链接，已停止。"
done

getent group cloudease-agent >/dev/null || groupadd --system cloudease-agent
if ! id -u cloudease-agent >/dev/null 2>&1; then
    useradd --system --gid cloudease-agent --no-create-home --shell "$(command -v nologin || echo /bin/false)" cloudease-agent
fi
[ "$(id -u cloudease-agent)" -ne 0 ] || die "服务账户不得拥有 root 权限。"
install -d -m 0755 /opt/cloudease-lite
install -d -m 0750 -o root -g cloudease-agent /etc/cloudease-lite
# Stage all configuration before stopping an existing Agent.
printf '%s\n' "$token" > "$work/agent-token"
unset token
printf 'CLOUDEASE_SERVER=%s\n' "$server" > "$work/agent.env"
cat > "$work/cloud-agent.service" <<'SERVICE'
[Unit]
Description=CloudEase Lite host monitoring client
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=cloudease-agent
Group=cloudease-agent
EnvironmentFile=/etc/cloudease-lite/agent.env
ExecStart=/opt/cloudease-lite/cloud-agent --server ${CLOUDEASE_SERVER} --token-file /etc/cloudease-lite/agent-token
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
CapabilityBoundingSet=
MemoryMax=128M
[Install]
WantedBy=multi-user.target
SERVICE
if [ "$container_monitor" = true ]; then
    # Explicit opt-in only: these directives can require mount namespaces or seccomp.
    # Keep NoNewPrivileges, an empty capability set and MemoryMax; never grant CAP_SYS_ADMIN.
    sed -i -e '/^ProtectSystem=/d' -e '/^ProtectHome=/d' -e '/^PrivateTmp=/d' \
        -e '/^PrivateDevices=/d' -e '/^ProtectKernelTunables=/d' -e '/^ProtectKernelModules=/d' \
        -e '/^ProtectControlGroups=/d' -e '/^RestrictSUIDSGID=/d' \
        -e '/^NoNewPrivileges=true$/a UMask=0077' \
        -e 's|^ExecStart=.*|& --website-config=|' "$work/cloud-agent.service"
fi
if [ "$management" = true ]; then
    # An explicit opt-in switches the client to root; no sudoers or Docker-group grant.
    [ ! -L /opt/cloud-ease ] || die "运维文件目录不能是符号链接。"
    if [ -e /opt/cloud-ease ]; then
        [ -d /opt/cloud-ease ] && [ "$(stat -c %u /opt/cloud-ease)" -eq 0 ] || die "运维文件目录必须由 root 所有。"
        mode=$(stat -c %a /opt/cloud-ease)
        [ "$((8#$mode & 0022))" -eq 0 ] || die "运维文件目录不能允许其他用户写入。"
    else
        install -d -m 0750 -o root -g root /opt/cloud-ease
    fi
    sed -i -e 's/^User=cloudease-agent$/User=root/' -e 's/^Group=cloudease-agent$/Group=root/' \
        -e 's|^ExecStart=.*|& --management|' -e '/^ProtectSystem=strict$/a ReadWritePaths=/opt/cloud-ease' "$work/cloud-agent.service"
    echo "已明确启用运维管理：客户端使用 root，仅执行内置白名单操作。"
    if [ -f /etc/cloudease-lite/websites.json ] && [ ! -L /etc/cloudease-lite/websites.json ]; then
        printf 'CLOUDEASE_WEBSITE_CONFIG=/etc/cloudease-lite/websites.json\n' >> "$work/agent.env"
        echo "检测到主机网站白名单，将由 v0.3 客户端校验后启用。自定义写入目录请保留 systemd drop-in 配置。"
    fi
fi
if systemctl is-active --quiet cloud-agent; then systemctl stop cloud-agent; fi
install -m 0755 "$work/cloud-agent" /opt/cloudease-lite/cloud-agent
install -m 0640 -o root -g cloudease-agent "$work/agent-token" /etc/cloudease-lite/agent-token
install -m 0640 -o root -g cloudease-agent "$work/agent.env" /etc/cloudease-lite/agent.env
install -m 0644 "$work/cloud-agent.service" /etc/systemd/system/cloud-agent.service
systemctl daemon-reload
systemctl enable --now cloud-agent
sleep 2
systemctl is-active --quiet cloud-agent || die "服务启动失败，请运行 journalctl -u cloud-agent -n 30 --no-pager 查看日志。"
echo "安装完成，监控客户端已启动并设置开机启动，每 4 秒上报一次。"
echo "查看状态：systemctl status cloud-agent --no-pager"
echo "查看日志：journalctl -u cloud-agent -n 20 --no-pager"
