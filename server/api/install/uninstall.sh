#!/bin/bash
# Remove only files created by the CloudEase Lite automatic client installer.
set -euo pipefail
die() { echo "卸载失败：$*" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || die "请在被监控服务器上使用 root 执行。"
fingerprint=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --key-fingerprint) [ "$#" -ge 2 ] || die "缺少密钥校验值。"; fingerprint="$2"; shift 2 ;;
        *) die "未知参数：$1" ;;
    esac
done
[[ "$fingerprint" =~ ^[a-f0-9]{64}$ ]] || die "请完整复制控制台的卸载命令。"
command -v systemctl >/dev/null && [ -d /run/systemd/system ] || die "此脚本用于通过自动安装命令安装的 systemd 客户端。"
for dir in /etc/cloudease-lite /opt/cloudease-lite; do
    [ ! -L "$dir" ] || die "安装目录是符号链接，已停止操作。"
done
key_file=/etc/cloudease-lite/agent-token
binary=/opt/cloudease-lite/cloud-agent
unit=/etc/systemd/system/cloud-agent.service
if [ ! -e "$key_file" ] && [ ! -L "$key_file" ]; then
    if [ ! -e "$binary" ] && [ ! -L "$binary" ] && [ ! -e "$unit" ] && [ ! -L "$unit" ] && [ ! -e /etc/cloudease-lite/agent.env ]; then
        echo "监控客户端已卸载或未通过自动安装命令安装。"
        exit 0
    fi
    die "缺少本机密钥文件，无法核对客户端，未删除任何文件。"
fi
[ -f "$key_file" ] && [ ! -L "$key_file" ] || die "密钥文件类型异常，已停止操作。"
key=$(tr -d '\r\n' < "$key_file")
[[ "$key" =~ ^[A-Za-z0-9_-]{43}$ ]] || die "本机密钥格式异常，已停止操作。"
actual=$(printf '%s' "$key" | sha256sum)
actual=${actual%% *}
unset key
[ "$actual" = "$fingerprint" ] || die "本机密钥与所选服务器不匹配。请确认服务器；若已轮换密钥，请先更新客户端配置。"
if [ -e "$unit" ] || [ -L "$unit" ]; then
    [ -f "$unit" ] && [ ! -L "$unit" ] || die "服务文件类型异常，已停止操作。"
    grep -qE '^ExecStart=/opt/cloudease-lite/cloud-agent([[:space:]]|$)' "$unit" || die "服务指向其他程序，已停止操作。"
    systemctl disable --now cloud-agent
elif systemctl is-active --quiet cloud-agent; then
    die "客户端使用非标准服务配置，请先手动停止服务。"
fi
systemctl is-active --quiet cloud-agent && die "服务仍在运行，未删除文件。"
# Never delete /opt/cloudease-lite recursively: it can also contain the control plane.
rm -f -- "$binary" "$key_file" /etc/cloudease-lite/agent.env "$unit"
systemctl daemon-reload
systemctl reset-failed cloud-agent 2>/dev/null || true
if [ -d /etc/cloudease-lite ] && [ -z "$(ls -A /etc/cloudease-lite)" ]; then rmdir /etc/cloudease-lite; fi
echo "监控客户端已卸载：服务已停止、开机启动已取消、程序和本机密钥已删除。"
echo "控制台中的连接密钥和服务器记录需在删除弹窗中确认删除。"
