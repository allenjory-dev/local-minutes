param(
    [string]$RuntimePath = (Join-Path $PSScriptRoot '../../../baseline-runtime'),
    [switch]$NoBrowser,
    [switch]$ShowErrors
)
$ErrorActionPreference='Stop'
try {
    & (Join-Path $PSScriptRoot 'Start-LocalOllama.ps1') -RuntimePath $RuntimePath
    & (Join-Path $PSScriptRoot 'Start-LocalMinutes.ps1') -RuntimePath $RuntimePath -NoBrowser:$NoBrowser
} catch {
    if($ShowErrors){
        Add-Type -AssemblyName System.Windows.Forms
        [void][System.Windows.Forms.MessageBox]::Show($_.Exception.Message,'Local Minutes startup','OK','Error')
    }
    throw
}
