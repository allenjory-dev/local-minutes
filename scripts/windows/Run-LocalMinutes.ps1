param([Parameter(Mandatory=$true)][string]$RuntimePath)
$ErrorActionPreference = 'Stop'
$runtime = (Resolve-Path -LiteralPath $RuntimePath).Path
$config = Get-Content -LiteralPath (Join-Path $runtime 'local-minutes-runtime.json') -Raw | ConvertFrom-Json
$env:PSModulePath = Join-Path $PSHOME 'Modules'
$env:HOST = '127.0.0.1'
$env:PORT = [string]$config.port
$env:APP_ENV = 'production'
$env:SECURE_COOKIES = 'false'
$env:ALLOWED_ORIGINS = "http://127.0.0.1:$($config.port),http://localhost:$($config.port)"
$env:DATABASE_PATH = Join-Path $runtime 'data/scriberr.db'
$env:UPLOAD_DIR = Join-Path $runtime 'data/uploads'
$env:TRANSCRIPTS_DIR = Join-Path $runtime 'data/transcripts'
$env:TEMP_DIR = Join-Path $runtime 'data/temp'
$env:WHISPERX_ENV = Join-Path $runtime 'model-envs'
$env:UV_CACHE_DIR = Join-Path $runtime 'uv-cache'
$env:UV_PYTHON_INSTALL_DIR = Join-Path $runtime 'python'
$env:UV_PYTHON = [string]$config.python
$env:HF_HOME = Join-Path $runtime 'hf-cache'
$env:TORCH_HOME = Join-Path $runtime 'torch-cache'
$env:XDG_CACHE_HOME = Join-Path $runtime 'cache'
$env:NLTK_DATA = Join-Path $runtime 'nltk-data'
$env:PYANNOTE_METRICS_ENABLED = '0'
$env:HF_HUB_DISABLE_TELEMETRY = '1'
$env:DO_NOT_TRACK = '1'
$env:PYTORCH_CUDA_VERSION = 'cu126'
$env:SCRIBERR_STARTUP_MODELS = 'whisperx'
# Use provisioned packages/models only. Installation is a separate deliberate action.
$env:UV_NO_SYNC = '1'
$env:UV_OFFLINE = '1'
$env:HF_HUB_OFFLINE = '1'
$env:TRANSFORMERS_OFFLINE = '1'
$env:OPENAI_API_KEY = ''
$env:HF_TOKEN = ''
$env:PATH = (Join-Path $runtime 'tools') + ';' + $env:PATH
Set-Location $runtime
$python = Join-Path $runtime 'model-envs/WhisperX/.venv/Scripts/python.exe'
$executable = Join-Path $runtime $config.executable
foreach ($required in @($python,$executable,$env:UV_PYTHON,(Join-Path $runtime 'tools/ffmpeg.exe'))) {
    if (-not (Test-Path -LiteralPath $required -PathType Leaf)) { throw "Required local file missing: $required. No installation was attempted." }
}
& $python -B (Join-Path $PSScriptRoot 'check-local-runtime.py')
if ($LASTEXITCODE -ne 0) { throw 'Local GPU environment check failed. No dependencies were changed.' }
# Cached Community-1 works without a real credential. The adapter requires presence;
# use a clearly non-secret marker, avoiding token decryption during ordinary startup.
$env:HF_TOKEN = 'hf_LOCAL_OFFLINE_CACHE_ONLY'
& $executable
exit $LASTEXITCODE
