param([Parameter(Mandatory=$true)][string]$KeyFingerprint)
$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run this command as Administrator on the monitored server.'
}
if ($KeyFingerprint -cnotmatch '^[a-f0-9]{64}$') { throw 'Copy the complete uninstall command.' }
$installDir = [IO.Path]::GetFullPath((Join-Path $env:ProgramData 'CloudEaseLite'))
$binary = Join-Path $installDir 'cloud-agent.exe'
$keyFile = Join-Path $installDir 'agent-token'
$taskName = 'CloudEaseLite-Agent'
foreach ($path in @($installDir, $binary, $keyFile)) {
    if (Test-Path -LiteralPath $path) {
        $item = Get-Item -LiteralPath $path -Force
        if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Unexpected link in the installation path. No changes made.' }
    }
}
$task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if (!(Test-Path -LiteralPath $keyFile -PathType Leaf)) {
    if (!(Test-Path -LiteralPath $binary) -and !$task) {
        Write-Host 'The monitoring client is already removed or was not installed with the automatic installer.'
        exit 0
    }
    throw 'Local connection key is missing. Cannot verify this client. No changes made.'
}
$key = [IO.File]::ReadAllText($keyFile).Trim()
if ($key -cnotmatch '^[A-Za-z0-9_-]{43}$') { throw 'Invalid local connection key. No changes made.' }
$sha = [Security.Cryptography.SHA256]::Create()
try { $actual = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($key)))).Replace('-', '').ToLowerInvariant() }
finally { $sha.Dispose(); Remove-Variable key }
if ($actual -cne $KeyFingerprint) {
    throw 'Local connection key does not match the selected server. Check the machine; update client configuration if the key was rotated. No changes made.'
}
if ($task) {
    foreach ($action in $task.Actions) {
        if (!$action.Execute -or [IO.Path]::GetFullPath($action.Execute) -ine $binary) {
            throw 'Startup task points to another program. No changes made.'
        }
    }
    Stop-ScheduledTask -TaskName $taskName
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
}
# Stop only the executable inside this verified installation, never processes by name.
Get-CimInstance Win32_Process -Filter "Name = 'cloud-agent.exe'" | ForEach-Object {
    if ($_.ExecutablePath -and [IO.Path]::GetFullPath($_.ExecutablePath) -ieq $binary) {
        Stop-Process -Id $_.ProcessId -Force -ErrorAction Stop
    }
}
foreach ($path in @($binary, $keyFile)) {
    $resolved = [IO.Path]::GetFullPath($path)
    if ([IO.Path]::GetDirectoryName($resolved) -ine $installDir) { throw 'Unexpected removal path.' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Force }
}
# Keep any unrelated files in the installation directory.
if ((Test-Path -LiteralPath $installDir) -and !(Get-ChildItem -LiteralPath $installDir -Force | Select-Object -First 1)) {
    [IO.Directory]::Delete($installDir, $false)
}
Write-Host 'Monitoring client removed: startup task, program, and local connection key deleted.'
Write-Host 'Confirm deletion of the server record and connection key in the control panel.'
