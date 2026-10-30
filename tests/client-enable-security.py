"""Run the local opt-in helper in an isolated filesystem with a mocked systemd.
No host service, credential, or system directory is read or modified.
"""
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
SCRIPT = (ROOT / 'server/api/install/enable-security.sh').read_text(encoding='utf-8')
BASH = sys.argv.pop(1) if len(sys.argv) > 1 else 'bash'

class EnableSecurity(unittest.TestCase):
    def run_helper(self, *, user='root', container=False, command=None, multiline=False,
                   restart_failure=False, overridden=False, conflict=False, old=False, writable=False):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            # Convert the Windows fixture path to Git Bash's POSIX path once.
            posix = subprocess.check_output([BASH, '-c', 'cd "$1" && pwd', 'fixture', str(root)], text=True).strip()
            for name in ['etc/systemd/system/cloud-agent.service.d', 'var/backups', 'opt/cloudease-lite']:
                (root / name).mkdir(parents=True, exist_ok=True)
            agent = root / 'opt/cloudease-lite/cloud-agent'
            agent.write_text('#!/usr/bin/env bash\nprintf "  -%s\\n" "' + ('once' if old else 'security') + '"\n', encoding='utf-8')
            # Paths are rewritten only in this in-memory test copy, never in production.
            script = SCRIPT
            for prefix in ['/etc', '/var', '/opt']:
                script = script.replace(prefix, posix + prefix)
            original = command or (posix + '/opt/cloudease-lite/cloud-agent --server ${CLOUDEASE_SERVER} --token-file /etc/cloudease-lite/agent-token --advertise-ip ${CLOUDEASE_IP} --management --website-config=/etc/cloudease-lite/websites.json')
            if command: original = original.replace('/opt/cloudease-lite/cloud-agent', posix + '/opt/cloudease-lite/cloud-agent')
            unit = '[Service]\nExecStart=' + original + (' \\\n  --extra' if multiline else '') + '\n'
            (root / 'original.conf').write_text(unit, encoding='utf-8', newline='\n')
            drop = root / 'etc/systemd/system/cloud-agent.service.d/90-security.conf'
            if conflict: drop.write_text('existing administrator config\n', encoding='utf-8')
            setup = r'''
set -euo pipefail
id() { echo 0; }
uname() { echo Linux; }
systemd-detect-virt() { [ "$FIXTURE_CONTAINER" = 1 ]; }
stat() { if [ "$2" = %u ]; then echo 0; elif [ "$FIXTURE_WRITABLE" = 1 ]; then echo 777; else echo 755; fi; }
sleep() { :; }
systemctl() {
  printf '%s\n' "$*" >> "$FIXTURE_ROOT/calls"
  case "$1" in
    is-active) return 0;;
    cat) cat "$FIXTURE_ROOT/original.conf";;
    show)
      if [ "$4" = User ]; then echo "$FIXTURE_USER"
      elif [ "$FIXTURE_OVERRIDDEN" = 1 ]; then cat "$FIXTURE_ROOT/original.conf"
      else cat "$FIXTURE_ROOT/original.conf" "$FIXTURE_ROOT/etc/systemd/system/cloud-agent.service.d/90-security.conf"; fi;;
    restart) [ "$FIXTURE_RESTART_FAILURE" != 1 ];;
    daemon-reload) return 0;;
    *) return 2;;
  esac
}
'''
            env = dict(os.environ, FIXTURE_ROOT=posix, FIXTURE_USER=user,
                       FIXTURE_CONTAINER=str(int(container)), FIXTURE_RESTART_FAILURE=str(int(restart_failure)),
                       FIXTURE_OVERRIDDEN=str(int(overridden)), FIXTURE_WRITABLE=str(int(writable)))
            subprocess.run([BASH, '-c', 'chmod +x "$1"', 'fixture', str(agent)], check=True)
            helper = root / 'helper.sh'
            helper.write_text(setup + script, encoding='utf-8', newline='\n')
            result = subprocess.run([BASH, '--noprofile', '--norc', str(helper)],
                                    env=env, capture_output=True, text=True, encoding='utf-8')
            calls = root / 'calls'
            return result, drop.read_text(encoding='utf-8') if drop.exists() else None, (root / 'original.conf').read_text(encoding='utf-8'), calls.read_text(encoding='utf-8') if calls.exists() else ''

    def test_preserves_original_parameters_and_adds_only_security(self):
        r, drop, original, calls = self.run_helper()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn(original.split('ExecStart=', 1)[1].strip() + ' --security', drop)
        self.assertNotIn('User=', drop)
        self.assertNotIn('Capability', drop)
        self.assertEqual(calls.count('restart cloud-agent'), 1)

    def test_ordinary_user_and_container_never_change_service(self):
        for kw in [dict(user='cloudease-agent'), dict(container=True), dict(old=True), dict(writable=True)]:
            r, drop, _, calls = self.run_helper(**kw)
            self.assertNotEqual(r.returncode, 0)
            self.assertIsNone(drop)
            self.assertNotIn('restart', calls)

    def test_missing_management_or_custom_command_refuses(self):
        for command in ['/opt/custom/client --management --token-file /etc/key', '/opt/cloudease-lite/cloud-agent --token-file /etc/key']:
            r, drop, _, calls = self.run_helper(command=command)
            self.assertNotEqual(r.returncode, 0)
            self.assertIsNone(drop)
            self.assertNotIn('restart', calls)

    def test_multiline_and_existing_override_are_not_overwritten(self):
        r, drop, _, calls = self.run_helper(multiline=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIsNone(drop)
        self.assertNotIn('restart', calls)
        r, drop, _, calls = self.run_helper(conflict=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(drop, 'existing administrator config\n')
        self.assertNotIn('restart', calls)

    def test_later_override_and_restart_failure_roll_back(self):
        for kw in [dict(overridden=True), dict(restart_failure=True)]:
            r, drop, _, calls = self.run_helper(**kw)
            self.assertNotEqual(r.returncode, 0)
            self.assertIsNone(drop)
            self.assertIn('已撤销', r.stderr)

    def test_already_enabled_is_idempotent(self):
        command = '/opt/cloudease-lite/cloud-agent --management --token-file /etc/key --security'
        r, drop, _, calls = self.run_helper(command=command)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIsNone(drop)
        self.assertNotIn('restart', calls)

if __name__ == '__main__': unittest.main()
