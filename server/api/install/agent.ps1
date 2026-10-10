param(
    [Parameter(Mandatory=$true)][string]$Server,
    [Alias('Token')][string]$Key,
    [string]$KeyFile
)
$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Open PowerShell as Administrator on the monitored server, then paste the command again.'
}
if (![Environment]::Is64BitOperatingSystem -or $env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') {
    throw 'This Windows monitoring client supports x64 Windows only.'
}
$uri = [Uri]$Server
if (!$uri.IsAbsoluteUri -or $uri.Scheme -notin @('http','https') -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') {
    throw 'Invalid control plane URL. Use http(s)://hostname or IP, optionally with a port.'
}
if ($uri.Scheme -eq 'http' -and !($uri.Host -in @('localhost','127.0.0.1','[::1]','::1'))) {
    throw 'Public installations require HTTPS. HTTP is allowed only on loopback.'
}
if ($KeyFile) {
    $keyItem=Get-Item -LiteralPath $KeyFile -Force
    if ($keyItem.PSIsContainer -or ($keyItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -or $keyItem.Length -gt 512) { throw 'Invalid key file.' }
    $keyAcl=Get-Acl -LiteralPath $keyItem.FullName
    $allowed=@($identity.User.Value,$keyAcl.GetOwner([Security.Principal.SecurityIdentifier]).Value,'S-1-5-18','S-1-5-19','S-1-5-32-544')
    foreach($entry in $keyAcl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
        if ($entry.AccessControlType -eq 'Allow' -and ($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::ReadData) -and $entry.IdentityReference.Value -notin $allowed) { throw 'Key file grants read access to another account.' }
    }
    $Key=[IO.File]::ReadAllText($keyItem.FullName).Trim()
}
if (!$Key) {
    $protectedKey=Read-Host 'Paste connection key (hidden)' -AsSecureString
    $Key=[Net.NetworkCredential]::new('', $protectedKey).Password
}
if ($Key -notmatch '^[A-Za-z0-9_-]{43}$') { throw 'Invalid connection key. Copy the complete installation command.' }
$Server = $Server.TrimEnd('/')
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$work = Join-Path ([IO.Path]::GetTempPath()) ('cloudease-' + [Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $work | Out-Null
$workAcl=New-Object Security.AccessControl.DirectorySecurity
$workAcl.SetAccessRuleProtection($true,$false)
foreach($sid in @($identity.User.Value,'S-1-5-18','S-1-5-32-544')) {
    $rule=New-Object Security.AccessControl.FileSystemAccessRule(
        (New-Object Security.Principal.SecurityIdentifier($sid)),
        [Security.AccessControl.FileSystemRights]::FullControl,
        ([Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit),
        [Security.AccessControl.PropagationFlags]::None,
        [Security.AccessControl.AccessControlType]::Allow)
    $workAcl.AddAccessRule($rule)
}
Set-Acl -LiteralPath $work -AclObject $workAcl
try {
    $headers = @{Authorization = 'Bearer ' + $Key}
    Write-Host 'Checking connection key and downloading monitoring client...'
    Invoke-RestMethod -Uri ($Server + '/api/agent/check') -Headers $headers -MaximumRedirection 0 | Out-Null
    $download = Join-Path $work 'cloud-agent.exe'
    Invoke-WebRequest -UseBasicParsing -Uri ($Server + '/api/agent/downloads/cloud-agent.exe') -Headers $headers -MaximumRedirection 0 -OutFile $download
    $expected=(Invoke-WebRequest -UseBasicParsing -Uri ($Server + '/api/agent/downloads/cloud-agent.exe/sha256') -Headers $headers -MaximumRedirection 0).Content.Trim()
    if ($expected -cnotmatch '^[a-f0-9]{64}$' -or (Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) {
        throw 'Downloaded program checksum mismatch. No program was executed.'
    }
    $stream=[IO.File]::OpenRead($download)
    try {
        $reader=New-Object IO.BinaryReader($stream)
        if ($reader.ReadUInt16() -ne 0x5a4d) { throw 'Invalid Windows executable.' }
        $stream.Position=0x3c
        $offset=$reader.ReadInt32()
        if ($offset -lt 64 -or $offset -gt ($stream.Length - 6)) { throw 'Invalid Windows executable header.' }
        $stream.Position=$offset
        if ($reader.ReadUInt32() -ne 0x4550 -or $reader.ReadUInt16() -ne 0x8664) { throw 'Expected a Windows x64 program.' }
    } finally { $stream.Dispose() }
    $installDir = Join-Path $env:ProgramData 'CloudEaseLite'
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    foreach($path in @($installDir,(Join-Path $installDir 'cloud-agent.exe'),(Join-Path $installDir 'agent-token'))) {
        if (Test-Path -LiteralPath $path) {
            $item=Get-Item -LiteralPath $path -Force
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Unexpected link in installation path.' }
        }
    }
    $owner=(Get-Acl -LiteralPath $installDir).GetOwner([Security.Principal.SecurityIdentifier]).Value
    $current=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    if ($owner -notin @($current,'S-1-5-18','S-1-5-32-544')) { throw 'Installation directory belongs to another account.' }
    $dirAcl = New-Object Security.AccessControl.DirectorySecurity
    $dirAcl.SetAccessRuleProtection($true, $false)
    $dirAcl.SetOwner((New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')))
    foreach ($sid in @('S-1-5-18', 'S-1-5-32-544')) {
        $dirRule = New-Object Security.AccessControl.FileSystemAccessRule(
            (New-Object Security.Principal.SecurityIdentifier($sid)),
            [Security.AccessControl.FileSystemRights]::FullControl,
            ([Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit),
            [Security.AccessControl.PropagationFlags]::None,
            [Security.AccessControl.AccessControlType]::Allow)
        $dirAcl.AddAccessRule($dirRule)
    }
    $serviceRule=New-Object Security.AccessControl.FileSystemAccessRule(
        (New-Object Security.Principal.SecurityIdentifier('S-1-5-19')),
        [Security.AccessControl.FileSystemRights]::ReadAndExecute,
        ([Security.AccessControl.InheritanceFlags]::ContainerInherit -bor [Security.AccessControl.InheritanceFlags]::ObjectInherit),
        [Security.AccessControl.PropagationFlags]::None,
        [Security.AccessControl.AccessControlType]::Allow)
    $dirAcl.AddAccessRule($serviceRule)
    Set-Acl -LiteralPath $installDir -AclObject $dirAcl
    $taskName = 'CloudEaseLite-Agent'
    $previous = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    if ($previous) { Stop-ScheduledTask -TaskName $taskName }
    $agentPath = Join-Path $installDir 'cloud-agent.exe'
    $tokenPath = Join-Path $installDir 'agent-token'
    Copy-Item -LiteralPath $download -Destination $agentPath -Force
    $binaryAcl=New-Object Security.AccessControl.FileSecurity
    $binaryAcl.SetAccessRuleProtection($true,$false)
    foreach($sid in @('S-1-5-18','S-1-5-32-544')) {
        $binaryAcl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule(
            (New-Object Security.Principal.SecurityIdentifier($sid)),
            [Security.AccessControl.FileSystemRights]::FullControl,
            [Security.AccessControl.AccessControlType]::Allow)))
    }
    $binaryAcl.AddAccessRule((New-Object Security.AccessControl.FileSystemAccessRule(
        (New-Object Security.Principal.SecurityIdentifier('S-1-5-19')),
        [Security.AccessControl.FileSystemRights]::ReadAndExecute,
        [Security.AccessControl.AccessControlType]::Allow)))
    Set-Acl -LiteralPath $agentPath -AclObject $binaryAcl
    # The key is readable only by the service account, SYSTEM and Administrators.
    $acl = New-Object Security.AccessControl.FileSecurity
    $acl.SetAccessRuleProtection($true, $false)
    $acl.SetOwner((New-Object Security.Principal.SecurityIdentifier('S-1-5-32-544')))
    foreach ($sid in @('S-1-5-18', 'S-1-5-32-544')) {
        $rule = New-Object Security.AccessControl.FileSystemAccessRule(
            (New-Object Security.Principal.SecurityIdentifier($sid)),
            [Security.AccessControl.FileSystemRights]::FullControl,
            [Security.AccessControl.AccessControlType]::Allow)
        $acl.AddAccessRule($rule)
    }
    $serviceKeyRule=New-Object Security.AccessControl.FileSystemAccessRule(
        (New-Object Security.Principal.SecurityIdentifier('S-1-5-19')),
        [Security.AccessControl.FileSystemRights]::Read,
        [Security.AccessControl.AccessControlType]::Allow)
    $acl.AddAccessRule($serviceKeyRule)
    if (!(Test-Path -LiteralPath $tokenPath)) { New-Item -ItemType File -Path $tokenPath | Out-Null }
    Set-Acl -LiteralPath $tokenPath -AclObject $acl
    [IO.File]::WriteAllText($tokenPath, $Key, (New-Object Text.UTF8Encoding($false)))
    $arguments = '--server "' + $Server + '" --token-file "' + $tokenPath + '"'
    $action = New-ScheduledTaskAction -Execute $agentPath -Argument $arguments -WorkingDirectory $installDir
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -MultipleInstances IgnoreNew
    $taskPrincipal = New-ScheduledTaskPrincipal -UserId 'S-1-5-19' -LogonType ServiceAccount -RunLevel Limited
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Principal $taskPrincipal -Force | Out-Null
    Start-ScheduledTask -TaskName $taskName
    Write-Host 'Installed. Monitoring client starts now and automatically at boot, reporting every 4 seconds.'
    Write-Host 'Check status: Get-ScheduledTask -TaskName CloudEaseLite-Agent'
} finally {
    $tempRoot=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    $resolvedWork=[IO.Path]::GetFullPath($work)
    if (!$resolvedWork.StartsWith($tempRoot,[StringComparison]::OrdinalIgnoreCase) -or [IO.Path]::GetFileName($resolvedWork) -notmatch '^cloudease-[0-9a-f-]{36}$') { throw 'Unsafe temporary cleanup path.' }
    Remove-Item -LiteralPath $resolvedWork -Recurse -Force
}
