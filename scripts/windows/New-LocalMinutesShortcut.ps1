param(
    [string]$RuntimePath = (Join-Path $PSScriptRoot '../../../baseline-runtime'),
    [string]$ShortcutDirectory = [Environment]::GetFolderPath('Desktop')
)
$ErrorActionPreference = 'Stop'
$runtime = (Resolve-Path -LiteralPath $RuntimePath).Path
$directory = (Resolve-Path -LiteralPath $ShortcutDirectory).Path
$launcher = Join-Path $PSScriptRoot 'Start-LocalMinutes.ps1'
$powershell = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
$shortcutPath = Join-Path $directory 'Local Minutes.lnk'
$arguments = '-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "{0}" -RuntimePath "{1}" -ShowErrors' -f $launcher,$runtime
$shell = New-Object -ComObject WScript.Shell
try {
    $shortcut = $shell.CreateShortcut($shortcutPath)
    if (Test-Path -LiteralPath $shortcutPath) {
        if ($shortcut.TargetPath -ne $powershell -or $shortcut.Arguments -ne $arguments) {
            throw 'A different Local Minutes shortcut already exists; it was preserved.'
        }
    }
    $shortcut.TargetPath = $powershell
    $shortcut.Arguments = $arguments
    $shortcut.WorkingDirectory = $runtime
    $shortcut.Description = 'Start Local Minutes locally and open its recording library'
    $shortcut.WindowStyle = 7
    $shortcut.Save()
    $saved = $shell.CreateShortcut($shortcutPath)
    if ($saved.TargetPath -ne $powershell -or $saved.Arguments -ne $arguments) { throw 'Shortcut read-back did not match.' }
    Write-Output "Local Minutes shortcut saved and verified: $shortcutPath"
} finally {
    [void][Runtime.InteropServices.Marshal]::ReleaseComObject($shell)
}
