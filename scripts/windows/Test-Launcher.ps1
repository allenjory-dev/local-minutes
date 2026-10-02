# No server is started or stopped. These tests exercise duplicate/conflict guards.
$ErrorActionPreference = 'Stop'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('local-minutes-launcher-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
$testExe = Join-Path $testRoot 'test-server.exe'
[IO.File]::WriteAllText($testExe, 'test fixture - not executable')
@{executable='test-server.exe';port=18787} | ConvertTo-Json | Set-Content (Join-Path $testRoot 'local-minutes-runtime.json')
$global:localMinutesTestMode = 'running'
$global:localMinutesTestLaunchCalls = 0
function Get-NetTCPConnection {
    param($State,$LocalPort,$ErrorAction)
    if ($global:localMinutesTestMode -eq 'pending') { return }
    [pscustomobject]@{OwningProcess=123;LocalAddress='127.0.0.1'}
}
function Get-Process {
    param($Id,$ErrorAction)
    $path = $testExe
    if ($global:localMinutesTestMode -eq 'foreign') { $path = 'C:\another-application.exe' }
    [pscustomobject]@{Id=123;Path=$path;StartTime=[datetime]'2026-01-01T00:00:00Z'}
}
function Invoke-RestMethod {
    param($Uri,$TimeoutSec,$Proxy)
    if ($global:localMinutesTestMode -eq 'unhealthy') { throw 'test health failure' }
    [pscustomobject]@{status='healthy'}
}
function Start-Process { $global:localMinutesTestLaunchCalls++; throw 'Unexpected process launch in guard test' }
$launcher = Join-Path $PSScriptRoot 'Start-LocalMinutes.ps1'
try {
    $output = & $launcher -RuntimePath $testRoot -NoBrowser
    if ($output -notmatch 'already running') { throw 'Existing healthy instance was not reused' }
    foreach ($case in @(@('foreign','belongs to another application'),@('unhealthy','not responding'),@('pending','previous Local Minutes launch is still active'))) {
        $global:localMinutesTestMode = $case[0]
        if ($global:localMinutesTestMode -eq 'pending') {
            @{worker_pid=123;worker_started_utc=([datetime]'2026-01-01T00:00:00Z').ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content (Join-Path $testRoot 'launcher-state.json')
        }
        $caught = $false
        try { & $launcher -RuntimePath $testRoot -NoBrowser } catch {
            if ($_.Exception.Message -notlike "*$($case[1])*") { throw }
            $caught = $true
        }
        if (-not $caught) { throw "Guard did not reject $($case[0])" }
    }
    if ($global:localMinutesTestLaunchCalls -ne 0) { throw 'Guard test unexpectedly started a process' }
    Write-Output 'PASS: existing healthy server reused; foreign listener, unhealthy instance and pending launch rejected; no processes started.'
} finally {
    # Remove only the explicit leaf fixtures created above, never a recursive path.
    foreach ($leaf in @('test-server.exe','local-minutes-runtime.json','launcher-state.json')) {
        $file = Join-Path $testRoot $leaf
        if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file }
    }
    Remove-Item -LiteralPath $testRoot
}
