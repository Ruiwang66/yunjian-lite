#!/bin/bash
# Run only in a disposable container with /fixtures mounted read-only.
set -euo pipefail
[ -f /.dockerenv ] && [ "${CLOUDEASE_UNINSTALL_FIXTURE:-}" = 1 ] || { echo "Disposable fixture container required."; exit 1; }
mkdir -p /run/systemd/system /usr/local/bin /opt/cloudease-lite /etc/cloudease-lite
cat >/usr/local/bin/systemctl <<'MOCK'
#!/bin/bash
echo "$*" >> /tmp/systemctl.calls
case "$1" in
 is-active) test -f /tmp/mock-active ;;
 disable) rm -f /tmp/mock-active ;;
 daemon-reload|reset-failed) exit 0 ;;
 *) exit 1 ;;
esac
MOCK
chmod 0755 /usr/local/bin/systemctl
export PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
key=$(printf '%043d' 0 | tr 0 A)
printf '%s\n' "$key" >/etc/cloudease-lite/agent-token
echo 'client binary' >/opt/cloudease-lite/cloud-agent
echo 'CLOUDEASE_SERVER=http://example.test' >/etc/cloudease-lite/agent.env
echo 'ExecStart=/opt/cloudease-lite/cloud-agent --token-file /etc/cloudease-lite/agent-token' >/etc/systemd/system/cloud-agent.service
echo 'control plane' >/opt/cloudease-lite/cloud-server
echo 'database' >/opt/cloudease-lite/cloudease.db
echo 'unrelated' >/etc/cloudease-lite/keep-me
touch /tmp/mock-active
fingerprint=$(printf '%s' "$key" | sha256sum); fingerprint=${fingerprint%% *}
bad=$(printf '%064d' 0)
before=$(sha256sum /opt/cloudease-lite/cloud-agent /etc/cloudease-lite/* /etc/systemd/system/cloud-agent.service)
if bash /fixtures/uninstall.sh --key-fingerprint "$bad"; then echo 'Wrong key accepted'; exit 1; fi
after=$(sha256sum /opt/cloudease-lite/cloud-agent /etc/cloudease-lite/* /etc/systemd/system/cloud-agent.service)
test "$before" = "$after"
test ! -e /tmp/systemctl.calls
# A matching key still cannot remove a unit configured to run another executable.
cp /etc/systemd/system/cloud-agent.service /tmp/unit
echo 'ExecStart=/usr/bin/other-program' >/etc/systemd/system/cloud-agent.service
if bash /fixtures/uninstall.sh --key-fingerprint "$fingerprint"; then echo 'Wrong service accepted'; exit 1; fi
test -e /opt/cloudease-lite/cloud-agent && test ! -e /tmp/systemctl.calls
cp /tmp/unit /etc/systemd/system/cloud-agent.service
bash /fixtures/uninstall.sh --key-fingerprint "$fingerprint"
for path in /opt/cloudease-lite/cloud-agent /etc/cloudease-lite/agent-token /etc/cloudease-lite/agent.env /etc/systemd/system/cloud-agent.service /tmp/mock-active; do test ! -e "$path"; done
test -s /opt/cloudease-lite/cloud-server && test -s /opt/cloudease-lite/cloudease.db && test -s /etc/cloudease-lite/keep-me
grep -qF 'disable --now cloud-agent' /tmp/systemctl.calls
bash /fixtures/uninstall.sh --key-fingerprint "$fingerprint"
echo 'LINUX_UNINSTALL_FIXTURE_OK: mismatched keys/services made no changes; matching client removed; control plane/database/unrelated files preserved; repeated removal succeeds.'
