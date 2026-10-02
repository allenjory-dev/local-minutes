# Local Minutes: desktop startup and daily use

Updated 2026-10-02. Step 1 is the scoped operational follow-up to the baseline. The application continues to display Scriberr branding. Local summaries, automatic meeting export to Obsidian/LifeBot, mobile capture and UI redesign remain separate work.

## Start and transcribe

1. Double-click the **Local Minutes** desktop shortcut. It starts the local server if needed and opens `http://127.0.0.1:8787` in the default browser. A repeated launch reuses the healthy instance. The shortcut runs without administrator privileges or a persistent terminal window.
2. Sign in to the existing local account if requested. Use **Add audio -> Upload Files** to import a recording.
3. Choose **Transcribe** for the recording, then **Start Transcription**. **Local Minutes - GPU Medium** is preselected. In a narrow window, swipe the recording card left to reveal Transcribe; the Advanced button opens separate manual settings.
4. Open the completed recording. Timeline View shows speaker labels/timestamps; selecting transcript words and choosing Listen seeks the audio.

The saved profile uses medium, CUDA, float16, batch 2, automatic speaker count, word alignment, auto language and the upstream Pyannote VAD. Generic Pyannote diarization resolves to Community-1 in the existing patched adapter. No token is saved in the profile. Automatic processing on upload remains off, so importing a large recording does not immediately start inference.

Closing the browser does not stop an active transcription or server. After a Windows restart, double-click the shortcut again. No service, scheduled task or automatic Windows startup entry was installed. Keep the source and sibling runtime folders in place; the shortcut references them. To relocate, update the local runtime configuration and regenerate the shortcut.

## Startup implementation

- `scripts/windows/Start-LocalMinutes.ps1`: validates the runtime executable, serializes repeated clicks, verifies the listening process belongs to that executable on loopback, checks health, then opens the library. It never kills an unknown listener. A long-running pending launch is detected through PID plus creation time, preventing a second launch after a timeout.
- `scripts/windows/Run-LocalMinutes.ps1`: applies process-local paths, runs an actual CUDA tensor preflight with the existing interpreter, then starts the server. Native Windows PowerShell uses its own built-in modules to avoid the previously encountered inherited-module conflict.
- `scripts/windows/New-LocalMinutesShortcut.ps1`: creates and reads back the desktop shortcut; refuses to replace a different existing shortcut.
- Private `baseline-runtime/local-minutes-runtime.json`: executable name (`scriberr-startup-1.exe`), port and existing Python interpreter path. Machine paths stay outside Git.
- `SCRIBERR_STARTUP_MODELS=whisperx`: initializes only WhisperX. Invalid selections fail before any adapter preparation; selected preparation failures abort startup and remain retryable. The unset setting preserves upstream behavior for other installations. This selects startup preparation, not an application-wide prohibition on choosing other engines.

Normal launch uses `UV_NO_SYNC=1`, `UV_OFFLINE=1`, `HF_HUB_OFFLINE=1` and `TRANSFORMERS_OFFLINE=1`. Existing dependencies and cached models are reused; missing models fail rather than being downloaded. Other languages/engines may need a separately approved provisioning step. These flags are not an OS firewall and do not disable every optional network feature in upstream.

Cached Community-1 was proven to work without the real Hugging Face credential. Ordinary startup uses an explicitly non-secret offline marker to satisfy the adapter's presence check; it does not decrypt the real download token. The protected DPAPI credential remains available for separately controlled provisioning. Never paste it into profiles or logs.

## Verification and limits

| Check | Result |
|---|---|
| Registry/service tests | PASS: selected-only setup, validation before side effects, trim/dedup, failure propagation and retry, changed selection, legacy unset behavior |
| Affected Go tests / Windows build | PASS: `go test ./internal/transcription/... ./internal/api/... -count=1`; portable Go, existing unchanged frontend bundle |
| Launcher guards | PASS: healthy instance reuse; foreign listener, unhealthy instance and pending launch rejected without starting a process |
| Real startup / second launch | PASS: only WhisperX prepared; second invocation reused the same server |
| Fresh server process restart | PASS: 4.17s measured launcher-to-ready; account/profile and saved result persisted |
| Playback after restart | PASS: reopened recording played from 0 to 30 seconds, ended paused with no media error; timeline retained both speaker labels |
| Saved profile through normal UI | PASS: 30s fixture, 12.24s GPU inference, 12 segments / 80 timed words / two speakers; no manual setting changes |
| Local scope / credentials | PASS: server bound to 127.0.0.1; profile/job token fields empty; real HF token not needed during startup/inference |
| Data and environment preservation | PASS: four prior audio/transcript records hash-identical; dependency files/package versions unchanged; original OpenWhispr executable/server/model hashes unchanged |
| Shortcut | PASS: installed and target/arguments read back; launcher exercised with browser opening suppressed during tests |
| Actual Windows reboot | PARTIAL: not performed to avoid interrupting the user's laptop; verify shortcut after next normal reboot |

The cached app run used offline flags, not a machine-wide network disconnect. Earlier cached engine tests were separately run under restricted networking. No claim is made that optional cloud features, webhooks or every external URL are blocked. Two previously documented unrelated full-suite failures remain; affected tests pass. This follow-up is not a long-meeting/noisy-room accuracy qualification. Spoken technical/code claims remain **UNVERIFIED**.

Private evidence: `baseline-runtime/operational-verification.json`, `before-operational-startup.json`, `openwhispr-after-operational-startup.json`, `logs/operational-go-tests.log`, `logs/operational-build.log`, timestamped `logs/local-minutes-*.{out,err}.log`, and database backup `data/backups/before-operational-startup-2026-10-02.db`.

## Troubleshooting and rollback

Startup errors are shown by the shortcut and logged under runtime/logs. Do not delete environments or run global pip/CUDA repairs. Missing-cache failures require explicit model provisioning. If another process owns the port, the launcher reports it and stops; identify it before taking action.

The earlier `scriberr-safety-3.exe` and `start-diarization.ps1` remain for rollback. Stop only the identified Local Minutes server when no jobs are active before switching builds; never run both against the same database/port. The selected-startup change introduces no schema migration. A consistent database backup was taken before operational startup changes.

Claude implemented the backend selection and regression tests after explicit user approval to send scoped source to Anthropic. Codex reviewed the patch, ran tests/builds, configured the launcher/profile and performed runtime verification. Audio, transcripts, credentials and private environment reports were excluded from that implementation request.
