$ErrorActionPreference='Stop'
$fixtureDir=Join-Path $PWD '.runtime\windows-removal-fixture'
$fixtureDir=[IO.Path]::GetFullPath($fixtureDir)
$allowed=[IO.Path]::GetFullPath((Join-Path $PWD '.runtime')) + '\'
if (!$fixtureDir.StartsWith($allowed,[StringComparison]::OrdinalIgnoreCase)) {throw 'Fixture path escaped workspace.'}
New-Item -ItemType Directory -Force -Path $fixtureDir | Out-Null
$source=Get-Content -LiteralPath 'server/api/install/uninstall.ps1' -Raw
# Bypass only the privilege check in a fixture copy; task and process APIs below are mocked.
$source=[regex]::Replace($source,'(?s)\$identity = .*?(?=if \(\$KeyFingerprint)','')
$fixtureScript=Join-Path $fixtureDir 'uninstall.ps1'
[IO.File]::WriteAllText($fixtureScript,$source,(New-Object Text.UTF8Encoding($false)))
$previousProgramData=$env:ProgramData
$env:ProgramData=$fixtureDir
$global:uninstallFixtureroot=Join-Path $fixtureDir 'CloudEaseLite'
New-Item -ItemType Directory -Force -Path $global:uninstallFixtureroot | Out-Null
$key='A'*43
[IO.File]::WriteAllText((Join-Path $global:uninstallFixtureroot 'agent-token'),$key)
[IO.File]::WriteAllText((Join-Path $global:uninstallFixtureroot 'cloud-agent.exe'),'fixture binary')
[IO.File]::WriteAllText((Join-Path $global:uninstallFixtureroot 'keep-me'),'unrelated data')
$sha=[Security.Cryptography.SHA256]::Create()
$fingerprint=([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($key)))).Replace('-','').ToLowerInvariant()
$sha.Dispose()
$global:uninstallFixturetaskExists=$true
$global:uninstallFixturewrongTask=$false
$global:uninstallFixturecalls=@()
$global:uninstallFixturekilled=@()
function Get-ScheduledTask {
 param($TaskName,$ErrorAction)
 if ($global:uninstallFixturetaskExists) {
  $execute=Join-Path $global:uninstallFixtureroot 'cloud-agent.exe'
  if($global:uninstallFixturewrongTask){$execute=Join-Path $fixtureDir 'unrelated.exe'}
  [PSCustomObject]@{Actions=@([PSCustomObject]@{Execute=$execute})}
 }
}
function Stop-ScheduledTask {param($TaskName); $global:uninstallFixturecalls += 'stop'}
function Unregister-ScheduledTask {param($TaskName,$Confirm); $global:uninstallFixturecalls += 'unregister'; $global:uninstallFixturetaskExists=$false}
function Get-CimInstance {
 param($ClassName,$Filter)
 [PSCustomObject]@{ExecutablePath=(Join-Path $global:uninstallFixtureroot 'cloud-agent.exe');ProcessId=1199}
 [PSCustomObject]@{ExecutablePath=(Join-Path $fixtureDir 'other\cloud-agent.exe');ProcessId=1200}
}
function Stop-Process {param($Id,[switch]$Force,$ErrorAction); $global:uninstallFixturekilled += $Id}
try {
 $failed=$false
 try { & $fixtureScript -KeyFingerprint ('0'*64) } catch {$failed=$true}
 if (!$failed -or $global:uninstallFixturecalls.Count -or !(Test-Path (Join-Path $global:uninstallFixtureroot 'cloud-agent.exe'))) {throw 'Wrong key modified installation.'}
 $global:uninstallFixturewrongTask=$true; $failed=$false
 try { & $fixtureScript -KeyFingerprint $fingerprint } catch {$failed=$true}
 if (!$failed -or $global:uninstallFixturecalls.Count) {throw 'Wrong task modified installation.'}
 $global:uninstallFixturewrongTask=$false
 & $fixtureScript -KeyFingerprint $fingerprint
 if ((Test-Path (Join-Path $global:uninstallFixtureroot 'cloud-agent.exe')) -or (Test-Path (Join-Path $global:uninstallFixtureroot 'agent-token'))) {throw 'Client files remain.'}
 if (!(Test-Path (Join-Path $global:uninstallFixtureroot 'keep-me'))) {throw 'Unrelated file removed.'}
 if ($global:uninstallFixturecalls -join ',' -ne 'stop,unregister' -or $global:uninstallFixturekilled -join ',' -ne '1199') {throw 'Task/process removal scope incorrect.'}
 'WINDOWS_UNINSTALL_FIXTURE_OK: wrong key/task refused; local files removed; unrelated files and process preserved. Task/process APIs mocked.'
} finally {$env:ProgramData=$previousProgramData}
