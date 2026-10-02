param(
    [string]$RuntimePath = (Join-Path $PSScriptRoot '../../../baseline-runtime'),
    [switch]$NoBrowser,
    [switch]$ShowErrors
)
$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSVersion.Major -eq 5) {
    # A host launched by PowerShell 7 must use its own built-in Windows modules.
    $env:PSModulePath = Join-Path $PSHOME 'Modules'
}
$lockHeld = $false
$launchLock = $null
try {
    $runtime = (Resolve-Path -LiteralPath $RuntimePath).Path
    $config = Get-Content -LiteralPath (Join-Path $runtime 'local-minutes-runtime.json') -Raw | ConvertFrom-Json
    $executable = (Resolve-Path -LiteralPath (Join-Path $runtime $config.executable)).Path
    $port = [int]$config.port
    if ($port -lt 1024 -or $port -gt 65535) { throw 'Invalid local port in runtime configuration.' }
    $url = "http://127.0.0.1:$port"
    # Serialize repeated clicks, including while the first server is warming up.
    $launchLock = New-Object Threading.Mutex($false, "Local\LocalMinutes-$port")
    try { $lockHeld = $launchLock.WaitOne(0) } catch [Threading.AbandonedMutexException] { $lockHeld = $true }
    if (-not $lockHeld) { Write-Output 'Local Minutes is already starting.'; return }

    function Get-LocalMinutesListener {
        $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue)
        if ($listeners.Count -eq 0) { return $null }
        $owners = @($listeners.OwningProcess | Sort-Object -Unique)
        if ($owners.Count -ne 1) { throw "Port $port is occupied. No process was stopped." }
        $ownerProcess = Get-Process -Id $owners[0] -ErrorAction Stop
        if (-not $ownerProcess.Path -or $ownerProcess.Path -ne $executable) {
            throw "Port $port belongs to another application or an older Local Minutes build. No process was stopped."
        }
        if (@($listeners | Where-Object { $_.LocalAddress -ne '127.0.0.1' }).Count) {
            throw 'The running server is not restricted to loopback. Check its configuration.'
        }
        return $ownerProcess
    }
    function Test-LocalMinutesHealth {
        try {
            $reply = Invoke-RestMethod -Uri "$url/health" -TimeoutSec 2 -Proxy $null
            return $reply.status -eq 'healthy'
        } catch { return $false }
    }

    $server = Get-LocalMinutesListener
    if ($server) {
        if (-not (Test-LocalMinutesHealth)) { throw 'Local Minutes is running but not responding. See the runtime logs.' }
        Write-Output "Local Minutes is already running (PID $($server.Id))."
    } else {
        $worker = Join-Path $PSScriptRoot 'Run-LocalMinutes.ps1'
        $statePath = Join-Path $runtime 'launcher-state.json'
        if (Test-Path -LiteralPath $statePath) {
            $previous = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
            $pending = Get-Process -Id $previous.worker_pid -ErrorAction SilentlyContinue
            if ($pending -and $pending.StartTime.ToUniversalTime().ToString('o') -eq $previous.worker_started_utc) {
                throw 'A previous Local Minutes launch is still active. Check its logs before starting again.'
            }
        }
        $logDirectory = Join-Path $runtime 'logs'
        New-Item -ItemType Directory -Force -Path $logDirectory | Out-Null
        $stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
        $outLog = Join-Path $logDirectory "local-minutes-$stamp.out.log"
        $errorLog = Join-Path $logDirectory "local-minutes-$stamp.err.log"
        $powershell = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
        # Paths cannot contain a quote on Windows; arguments are individually quoted.
        $arguments = '-NoProfile -ExecutionPolicy Bypass -File "{0}" -RuntimePath "{1}"' -f $worker,$runtime
        $workerProcess = Start-Process -FilePath $powershell -ArgumentList $arguments -WorkingDirectory $runtime -WindowStyle Hidden -RedirectStandardOutput $outLog -RedirectStandardError $errorLog -PassThru
        @{ worker_pid=$workerProcess.Id; worker_started_utc=$workerProcess.StartTime.ToUniversalTime().ToString('o'); log=$outLog } | ConvertTo-Json | Set-Content -LiteralPath $statePath
        $deadline = (Get-Date).AddSeconds(120)
        do {
            $workerProcess.Refresh()
            if ($workerProcess.HasExited) { throw "Local Minutes could not start. See $errorLog and $outLog" }
            $server = Get-LocalMinutesListener
            if ($server -and (Test-LocalMinutesHealth)) { break }
            Start-Sleep -Milliseconds 750
        } while ((Get-Date) -lt $deadline)
        if (-not $server -or -not (Test-LocalMinutesHealth)) {
            throw "Startup is taking longer than expected. See $outLog. The launcher has not stopped any process."
        }
        Write-Output "Local Minutes ready (PID $($server.Id)); $url"
    }
    if (-not $NoBrowser) { Start-Process $url }
} catch {
    if ($ShowErrors) {
        Add-Type -AssemblyName System.Windows.Forms
        [void][System.Windows.Forms.MessageBox]::Show($_.Exception.Message, 'Local Minutes startup', 'OK', 'Error')
    }
    throw
} finally {
    if ($lockHeld) { $launchLock.ReleaseMutex() }
    if ($launchLock) { $launchLock.Dispose() }
}
