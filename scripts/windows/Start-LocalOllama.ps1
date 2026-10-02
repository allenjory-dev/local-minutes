param([string]$RuntimePath = (Join-Path $PSScriptRoot '../../../baseline-runtime'))
$ErrorActionPreference='Stop'
if ($PSVersionTable.PSVersion.Major -eq 5) { $env:PSModulePath=Join-Path $PSHOME 'Modules' }
$runtime=(Resolve-Path -LiteralPath $RuntimePath).Path
$exe=(Resolve-Path -LiteralPath (Join-Path $runtime 'tools/ollama-v0.35.0/ollama.exe')).Path
$mutex=New-Object Threading.Mutex($false,'Local\LocalMinutes-Ollama-11434')
$held=$false
try {
    try {$held=$mutex.WaitOne(0)} catch [Threading.AbandonedMutexException] {$held=$true}
    if(-not $held){throw 'Local summary server is already starting.'}
    $listeners=@(Get-NetTCPConnection -State Listen -LocalPort 11434 -ErrorAction SilentlyContinue)
    if($listeners.Count){
        $owners=@($listeners.OwningProcess | Sort-Object -Unique)
        if($owners.Count -ne 1 -or @($listeners | Where-Object LocalAddress -ne '127.0.0.1').Count){throw 'Unexpected listener on Ollama port; nothing stopped.'}
        $existing=Get-Process -Id $owners[0]
        if($existing.Path -ne $exe){throw 'Ollama port belongs to another installation; nothing stopped.'}
        $version=Invoke-RestMethod 'http://127.0.0.1:11434/api/version' -TimeoutSec 3 -Proxy $null
        Write-Output "Local summary server already running: $($version.version)"
        return
    }
    $env:OLLAMA_HOST='127.0.0.1:11434'
    $env:OLLAMA_MODELS=Join-Path $runtime 'ollama-models'
    $env:OLLAMA_NO_CLOUD='1'
    $env:OLLAMA_NOHISTORY='1'
    $env:OLLAMA_DEBUG='0'
    $env:OLLAMA_DEBUG_LOG_REQUESTS='0'
    $env:OLLAMA_CONTEXT_LENGTH='8192'
    $env:OLLAMA_KEEP_ALIVE='0'
    $env:OLLAMA_NUM_PARALLEL='1'
    $env:OLLAMA_MAX_LOADED_MODELS='1'
    $env:OLLAMA_FLASH_ATTENTION='1'
    $stamp=Get-Date -Format 'yyyyMMdd-HHmmss-fff'
    $outLog=Join-Path $runtime "logs/ollama-$stamp.out.log"
    $errLog=Join-Path $runtime "logs/ollama-$stamp.err.log"
    $server=Start-Process -FilePath $exe -ArgumentList 'serve' -WorkingDirectory $runtime -WindowStyle Hidden -RedirectStandardOutput $outLog -RedirectStandardError $errLog -PassThru
    @{pid=$server.Id;started_utc=$server.StartTime.ToUniversalTime().ToString('o');error_log=$errLog} | ConvertTo-Json | Set-Content (Join-Path $runtime 'ollama-state.json')
    $deadline=(Get-Date).AddSeconds(25)
    do {
        $server.Refresh()
        if($server.HasExited){throw "Ollama exited. See $errLog"}
        try {$version=Invoke-RestMethod 'http://127.0.0.1:11434/api/version' -TimeoutSec 2 -Proxy $null;break} catch {Start-Sleep -Milliseconds 500}
    } while((Get-Date) -lt $deadline)
    if(-not $version){throw "Ollama startup pending; inspect $errLog before retrying."}
    Write-Output "Local summary server ready: $($version.version), PID $($server.Id)"
} finally {if($held){$mutex.ReleaseMutex()};$mutex.Dispose()}
