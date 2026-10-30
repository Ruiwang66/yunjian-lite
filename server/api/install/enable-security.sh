#!/usr/bin/env bash
# Local opt-in only. Preserve the existing root management service and its arguments.
set -euo pipefail
umask 077
die() { echo "未开启安全检测：$*" >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || die "请在要检测的 Linux 主机上使用 root 执行。"
[ "$(uname -s)" = Linux ] || die "只读安全检测目前仅支持 Linux。"
command -v systemctl >/dev/null || die "需要 systemd 管理现有客户端。"
command -v systemd-detect-virt >/dev/null || die "无法确认是否为容器，请先人工核对主机环境。"
if systemd-detect-virt --container --quiet; then
    die "容器客户端请保留仅监控模式，不会开启安全检测或提升容器权限。"
fi
systemctl is-active --quiet cloud-agent || die "请先安装并启动 v0.4 Linux 客户端。"
help_text=$(/opt/cloudease-lite/cloud-agent --help 2>&1) || die "无法确认客户端支持安全检测，请先升级至 v0.4。"
[[ "$help_text" =~ (^|$'\n')[[:space:]]+-security([[:space:]]|$) ]] || die "此客户端尚不支持安全检测，请先升级至 v0.4。"
run_user=$(systemctl show cloud-agent -p User --value)
[[ "$run_user" = root || "$run_user" = 0 ]] || die "当前客户端未授权 root 运维管理。请在 Linux 安装设置中明确勾选运维管理与只读安全检测。"
command_line=$(systemctl cat cloud-agent --no-pager | awk '
    { sub(/\r$/, "") }
    /^\[/ { service = ($0 == "[Service]") }
    service && /^[ \t]*ExecStart[ \t]*=/ {
        value = $0; sub(/^[ \t]*ExecStart[ \t]*=[ \t]*/, "", value)
        if (value ~ /\\[ \t]*$/) unsupported = 1
        command = value
    }
    END { if (unsupported) exit 2; print command }
') || die "服务使用多行启动命令，请保留原参数并由管理员手动追加 --security。"
[[ "$command_line" = /opt/cloudease-lite/cloud-agent\ * ]] || die "服务启动路径不是标准监控客户端，未修改自定义服务。"
[[ "$command_line" =~ (^|[[:space:]])--management([[:space:]]|$) ]] || die "服务尚未启用运维管理，未更改原权限。"
[[ "$command_line" =~ (^|[[:space:]])--token-file([[:space:]]|=) ]] || die "请先将客户端升级为使用本地密钥文件的 v0.4 安装方式。"
if [[ "$command_line" =~ (^|[[:space:]])--security([[:space:]]|$) ]]; then
    echo "安全检测已开启。返回安全中心刷新，再点击“开始只读检测”。"
    exit 0
fi
secure_directory() {
    [ -d "$1" ] && [ ! -L "$1" ] && [ "$(stat -c %u "$1")" -eq 0 ] || die "目录必须由 root 所有且不能是链接：$1"
    local mode
    mode=$(stat -c %a "$1")
    [ "$((8#$mode & 0022))" -eq 0 ] || die "目录不能允许其他账户写入：$1"
}
secure_directory /etc
secure_directory /etc/systemd
secure_directory /etc/systemd/system
drop_dir=/etc/systemd/system/cloud-agent.service.d
if [ ! -e "$drop_dir" ] && [ ! -L "$drop_dir" ]; then mkdir -m 0755 "$drop_dir"; fi
secure_directory "$drop_dir"
drop_file="$drop_dir/90-security.conf"
[ ! -e "$drop_file" ] && [ ! -L "$drop_file" ] || die "已有 90-security.conf，请先人工检查；不会覆盖现有配置。"
secure_directory /var
secure_directory /var/backups
if [ ! -e /var/backups/cloudease-lite ] && [ ! -L /var/backups/cloudease-lite ]; then
    mkdir -m 0700 /var/backups/cloudease-lite
fi
secure_directory /var/backups/cloudease-lite
backup=$(mktemp -d /var/backups/cloudease-lite/security-enable.XXXXXXXX)
systemctl cat cloud-agent --no-pager > "$backup/cloud-agent-before.conf"
created=false
restart_attempted=false
rollback() {
    local status=$?
    if [ "$status" -ne 0 ] && [ "$created" = true ]; then
        rm -f -- "$drop_file"
        systemctl daemon-reload || true
        if [ "$restart_attempted" = true ]; then systemctl restart cloud-agent || true; fi
        echo "启用未完成，已撤销本次新增配置；原配置备份：$backup" >&2
    fi
}
trap rollback EXIT
# Noclobber prevents replacing a file created after the checks above.
set -o noclobber
exec 3> "$drop_file"
created=true
set +o noclobber
printf '[Service]\nExecStart=\nExecStart=%s --security\n' "$command_line" >&3
exec 3>&-
systemctl daemon-reload
effective=$(systemctl show cloud-agent -p ExecStart --value)
[[ "$effective" =~ (^|[[:space:]])--security([[:space:];]|$) ]] || die "后续服务覆盖配置阻止了启用，原服务将保持不变。"
restart_attempted=true
systemctl restart cloud-agent
sleep 2
systemctl is-active --quiet cloud-agent || die "客户端未能正常启动。"
echo "只读安全检测已开启，配置备份：$backup"
echo "返回安全中心刷新，再点击“开始只读检测”。日志读取仍需单独的本地白名单。"
