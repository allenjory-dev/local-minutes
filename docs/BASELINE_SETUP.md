# Baseline setup and handoff

This is a qualification installation, not a finished Local Minutes release. The application still displays Scriberr branding, intentionally. Read PRODUCT_SPEC.md before making changes.

## Layout

The source repository is `local-minutes/`; its sibling `baseline-runtime/` holds the executable, portable toolchain, isolated model environments, caches, test fixture, logs and private evidence. Neither directory replaces an existing Whisper installation.

The Windows server was built from upstream application commit `a353078fd96b8aca4002681813524b7397c90df1`, after building its frontend. Source code was not patched. The latest published v1.2.0 binary was not used because it predates inspected fixes.

## Run

From the workspace parent, use Windows PowerShell:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\baseline-runtime\start-baseline.ps1
```

The execution-policy setting applies only to that process. The script sets process-local paths and binds HTTP to `127.0.0.1:8787`. It does not install a Windows service or edit persistent PATH. Open http://127.0.0.1:8787. First-time local admin registration is required; never share that password or commit it.

After the approved 2026-10-02 safety follow-up, use `baseline-runtime/start-safety.ps1` for the current patched executable, `scriberr-safety-1.exe`. The original baseline executable/script are retained for rollback; do not run both servers together. A consistent private database backup was made before the additive source-path migration. See SAFETY_PATCH.md for tested changes and remaining limitations.

Runtime scripts are machine-local because they point to the inspected existing Python interpreter. For another machine, establish its own inventory and installation procedure rather than copying those absolute paths.

Startup can take time and logs adapter failures while still reporting healthy. Confirm the selected engine actually works. Do not start a second server on the same port. Do not terminate unrelated Python/OpenWhispr processes.

## Tested engine command

The local `test-whisperx.ps1` uses a disposable 30-second two-voice fixture, tiny English model, CUDA float16, batch size 2, Silero VAD and word alignment. `-Offline` sets Hugging Face and Transformers offline mode. It is an engine test, separate from the app's upload/queue/playback path.

## Pending handoff

- Admin setup, short WAV import and app-level CUDA transcription completed on 2026-10-02. Transcript survived browser reload; full baseline acceptance remains pending.
- Playback after first registration failed with an audio HTTP 401. Register omits the media access cookie that Login/Refresh set. Ordinary sign-out/sign-in restored playback without code changes. Text selection then Listen sought to the exact stored word timestamp. Transcript and timestamp-linked note survived a server restart. Timeline view displayed segment timestamps.
- Pyannote access is not configured. Its gated model requires the user's own account/access acceptance. Never paste tokens into chat; upstream argument logging is a known risk.
- Sortformer/NeMo native setup failed on a C++ build requirement. No system build tools, Docker or WSL were installed.
- Ollama was not present; local summaries remain untested.
- Consult local docs/BASELINE_TEST_RESULTS.md for exact statuses; docs/LOCAL_ENVIRONMENT.md and detailed runtime logs stay out of the public repository.

## Development boundary

origin is the user's fork; upstream fetches original Scriberr and its local push URL is disabled. develop contains project documentation. main remains the fork's upstream baseline. No UI redesign, Android work, personal integration or engine replacement has begun.

Fixing original-audio retention and token-safe logging, pinning dependencies, adding portable sessions and implementing UNVERIFIED claims all require the next approved development scope. Do not interpret a successful engine test as approval to proceed with those features.
