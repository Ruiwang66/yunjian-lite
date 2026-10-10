#!/usr/bin/env bash
# CloudEase Lite: install a Linux Agent and enable it at boot.
set -euo pipefail
umask 077
die() { echo "安装失败：$*" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || die "请在被监控服务器上使用 root 执行。"
[ "$(uname -s)" = Linux ] || die "此脚本仅支持 Linux。"
command -v systemctl >/dev/null && [ -d /run/systemd/system ] || die "需要正在运行的 systemd。"
server= token= key_file=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --server) [ "$#" -ge 2 ] || die "--server 缺少地址"; server="$2"; shift 2 ;;
        --key-file) [ "$#" -ge 2 ] || die "缺少密钥文件"; key_file="$2"; shift 2 ;;
        --key|--token) [ "$#" -ge 2 ] || die "缺少连接密钥"; token="$2"; shift 2 ;;
        *) die "未知参数：$1" ;;
    esac
done
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
