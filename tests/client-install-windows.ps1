$ErrorActionPreference='Stop'
$fixtureRoot=[IO.Path]::GetFullPath((Join-Path (Join-Path $PWD '.runtime') ('windows-install-fixture-' + [Guid]::NewGuid().ToString())))
$allowedRoot=[IO.Path]::GetFullPath((Join-Path $PWD '.runtime')).TrimEnd('\')+'\'
if (!$fixtureRoot.StartsWith($allowedRoot,[StringComparison]::OrdinalIgnoreCase)) {throw 'Fixture path escaped workspace.'}
New-Item -ItemType Directory -Force -Path $fixtureRoot | Out-Null
$source=Get-Content -LiteralPath 'server/api/install/agent.ps1' -Raw
# Only bypass the elevated-role requirement in the fixture. Never install a real task.
$source=[regex]::Replace($source,'(?s)if \(!\$principal\.IsInRole\(.*?(?=if \(!\[Environment\])','')
$fixtureScript=Join-Path $fixtureRoot 'install.ps1'
[IO.File]::WriteAllText($fixtureScript,$source,(New-Object Text.UTF8Encoding($false)))
$previousProgramData=$env:ProgramData
$previousTemp=$env:TEMP
$previousTmp=$env:TMP
$env:ProgramData=$fixtureRoot
$env:TEMP=$fixtureRoot
$env:TMP=$fixtureRoot
$global:installFixtureCalls=@()
$global:installFixtureACLs=@()
$global:installFixtureChecksumBad=$false
$global:installFixtureBinaryBad=$false
$global:installFixtureSource=Join-Path $PWD 'bin\downloads\cloud-agent.exe'
function Read-Host {param($Prompt,[switch]$AsSecureString);ConvertTo-SecureString ('A'*43) -AsPlainText -Force}
function Invoke-RestMethod {param($Uri,$Headers,$MaximumRedirection);$global:installFixtureCalls+='check';[PSCustomObject]@{ok=$true}}
function Invoke-WebRequest {
 param([switch]$UseBasicParsing,$Uri,$Headers,$MaximumRedirection,$OutFile)
 if($MaximumRedirection -ne 0){throw 'Redirect enabled'}
 if($Headers.Authorization -ne ('Bearer '+('A'*43))){throw 'Authorization missing'}
 if($OutFile){
  if($global:installFixtureBinaryBad){[IO.File]::WriteAllText($OutFile,'not a PE executable')}
  else{Copy-Item -LiteralPath $global:installFixtureSource -Destination $OutFile}
  $global:installFixtureCalls+='download'
 } else {
  $global:installFixtureCalls+='checksum'
  $sum=(Get-FileHash -LiteralPath $global:installFixtureSource -Algorithm SHA256).Hash.ToLowerInvariant()
  if($global:installFixtureChecksumBad){$sum='0'*64}
  if($global:installFixtureBinaryBad){$sha=[Security.Cryptography.SHA256]::Create();$sum=([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes('not a PE executable')))).Replace('-','').ToLowerInvariant();$sha.Dispose()}
  [PSCustomObject]@{Content=$sum}
 }
}
function Set-Acl {param($LiteralPath,$AclObject);$global:installFixtureACLs+=@([PSCustomObject]@{Path=$LiteralPath;Acl=$AclObject})}
function Get-ScheduledTask {param($TaskName,$ErrorAction);return $null}
function Stop-ScheduledTask {param($TaskName);throw 'Unexpected real task'}
function New-ScheduledTaskAction {param($Execute,$Argument,$WorkingDirectory);if($Argument.Contains('A'*43)){throw 'Key in task arguments'};[PSCustomObject]@{Execute=$Execute;Argument=$Argument}}
function New-ScheduledTaskTrigger {param([switch]$AtStartup);[PSCustomObject]@{AtStartup=$AtStartup}}
function New-ScheduledTaskSettingsSet {param([switch]$StartWhenAvailable,$ExecutionTimeLimit,$RestartCount,$RestartInterval,$MultipleInstances);[PSCustomObject]@{}}
function New-ScheduledTaskPrincipal {param($UserId,$LogonType,$RunLevel);if($UserId -ne 'S-1-5-19' -or $LogonType -ne 'ServiceAccount' -or $RunLevel -ne 'Limited'){throw 'Privileged service principal'};[PSCustomObject]@{UserId=$UserId}}
function Register-ScheduledTask {param($TaskName,$Action,$Trigger,$Settings,$Principal,[switch]$Force);$global:installFixtureCalls+='register'}
function Start-ScheduledTask {param($TaskName);$global:installFixtureCalls+='start'}
try {
 $failed=$false;try {& $fixtureScript -Server 'http://monitor.example'}catch{$failed=$true}
 if(!$failed -or $global:installFixtureCalls.Count){throw 'Public HTTP was used'}
 $global:installFixtureChecksumBad=$true
 $failed=$false;try{& $fixtureScript -Server 'https://monitor.example'}catch{$failed=$true}
 if(!$failed -or $global:installFixtureCalls -contains 'register' -or (Test-Path (Join-Path $fixtureRoot 'CloudEaseLite'))){throw 'Checksum failure modified installation'}
 $global:installFixtureCalls=@();$global:installFixtureChecksumBad=$false;$global:installFixtureBinaryBad=$true
 $failed=$false;try{& $fixtureScript -Server 'https://monitor.example'}catch{$failed=$true}
 if(!$failed -or $global:installFixtureCalls -contains 'register'){throw 'Invalid executable accepted'}
 $global:installFixtureBinaryBad=$false;$global:installFixtureCalls=@()
 & $fixtureScript -Server 'https://monitor.example'
 if($global:installFixtureCalls -join ',' -ne 'check,download,checksum,register,start'){throw 'Unexpected installer flow'}
 $keyAcl=$global:installFixtureACLs | Where-Object {$_.Path -eq (Join-Path $fixtureRoot 'CloudEaseLite\agent-token')} | Select-Object -Last 1
 $rules=$keyAcl.Acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])
 $allowed=@('S-1-5-18','S-1-5-19','S-1-5-32-544')
 foreach($rule in $rules){if($rule.IdentityReference.Value -notin $allowed){throw 'Unexpected key reader'}}
 $service=$rules | Where-Object {$_.IdentityReference.Value -eq 'S-1-5-19'}
 if(!$service -or ($service.FileSystemRights -band [Security.AccessControl.FileSystemRights]::WriteData)){throw 'Service key permissions not read-only'}
 'WINDOWS_INSTALL_FIXTURE_OK: HTTP, hash mismatch and invalid PE rejected; hidden prompt and LocalService principal verified. Downloads/tasks/ACL application mocked, no program executed.'
} finally {$env:ProgramData=$previousProgramData;$env:TEMP=$previousTemp;$env:TMP=$previousTmp}
