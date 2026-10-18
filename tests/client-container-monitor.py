"""Exercise installer option guards and generated units without installing a service.

Run with Python 3 and bash (or pass the Git Bash executable as the first argument).
"""
import pathlib
import re
import subprocess
import sys
import tempfile
import unittest

SCRIPT = (pathlib.Path(__file__).resolve().parents[1] / "server/api/install/agent.sh").read_text(encoding="utf-8")
BASH = sys.argv.pop(1) if len(sys.argv) > 1 else "bash"
OPTIONS = SCRIPT[SCRIPT.index("server= token= key_file="):SCRIPT.index("server_pattern=")]
UNIT = SCRIPT[SCRIPT.index('cat > "$work/cloud-agent.service"'):SCRIPT.index('if [ "$management" = true ]; then')]


class ContainerMonitorInstaller(unittest.TestCase):
    def shell(self, fragment, arguments=(), virt=0, unit="LoadState=loaded\nDropInPaths=", show_status=0):
        setup = """set -euo pipefail
die() { echo "$*" >&2; exit 1; }
systemd-detect-virt() { return "$FIXTURE_VIRT"; }
systemctl() { printf '%s\\n' "$FIXTURE_UNIT"; return "$FIXTURE_SHOW_STATUS"; }
"""
        import os
        env = dict(os.environ, FIXTURE_VIRT=str(virt), FIXTURE_UNIT=unit, FIXTURE_SHOW_STATUS=str(show_status))
        with tempfile.TemporaryDirectory() as directory:
            env["FIXTURE_ROOT"] = pathlib.Path(directory).as_posix()
            # Keep override discovery away from the real machine's service directories.
            fragment = re.sub(r"/(?:etc|run|usr/lib|lib)/systemd/system/cloud-agent.service.d",
                              lambda match: '"${FIXTURE_ROOT}' + match.group() + '"', fragment)
            return subprocess.run([BASH, "--noprofile", "--norc", "-c", setup + fragment, "fixture", *arguments],
                                  env=env, capture_output=True, text=True, encoding="utf-8")

    def test_default_does_not_require_container_inspection(self):
        result = self.shell(OPTIONS, virt=1, show_status=99)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_conflict_both_argument_orders(self):
        for arguments in [("--management", "--container-monitor"), ("--container-monitor", "--management")]:
            result = self.shell(OPTIONS, arguments)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("不能同时", result.stderr)

    def test_only_identified_containers(self):
        result = self.shell(OPTIONS, ("--container-monitor",), virt=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("systemd", result.stderr)

    def test_first_install_missing_unit_is_allowed(self):
        result = self.shell(OPTIONS, ("--container-monitor",), unit="LoadState=not-found\nDropInPaths=", show_status=4)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_drop_in_and_inspection_failure_do_not_proceed(self):
        for unit, status in [("LoadState=loaded\nDropInPaths=/fixture/20-management.conf", 0), ("", 1)]:
            result = self.shell(OPTIONS, ("--container-monitor",), unit=unit, show_status=status)
            self.assertNotEqual(result.returncode, 0)

    def test_generated_units_preserve_authorized_boundaries(self):
        for container in ("false", "true"):
            result = self.shell('work=$(mktemp -d)\ntrap \'rm -rf -- "$work"\' EXIT\ncontainer_monitor=' + container +
                                '\n' + UNIT + '\ncat "$work/cloud-agent.service"\n')
            self.assertEqual(result.returncode, 0, result.stderr)
            fields = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
            self.assertEqual(fields["User"], "cloudease-agent")
            self.assertEqual(fields["Group"], "cloudease-agent")
            self.assertEqual(fields["NoNewPrivileges"], "true")
            self.assertEqual(fields["CapabilityBoundingSet"], "")
            self.assertEqual(fields["MemoryMax"], "128M")
            self.assertNotIn("--management", fields["ExecStart"])
            for directive in ("ProtectSystem", "ProtectHome", "PrivateTmp", "PrivateDevices", "ProtectKernelTunables",
                              "ProtectKernelModules", "ProtectControlGroups", "RestrictSUIDSGID"):
                self.assertEqual(directive in fields, container == "false")
            if container == "true":
                self.assertEqual(fields["UMask"], "0077")
                self.assertTrue(fields["ExecStart"].endswith(" --website-config="))


if __name__ == "__main__":
    unittest.main()
