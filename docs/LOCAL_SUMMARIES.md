# Local summary qualification - 2026-10-02

> **Update - summary reliability increment 1 (branch `feature/summary-reliability-v1`, not yet installed):** truthful generation status, attempts that never replace a saved summary, transcript-free logs, application-set UNVERIFIED/draft status and evidence-linked candidates checked against the stored transcript. See [SUMMARY_RELIABILITY.md](SUMMARY_RELIABILITY.md). It is synthetic-tested only; the qualification below is the record for the installed build and accuracy remains FAIL until re-qualified on the real model.

**Configuration works; accuracy gate FAIL.** Local GPU summaries are available as reviewable AI drafts. They are not accepted as reliable action extraction or verified technical information. This follow-up was authorized after operational step 1. No application source, transcription engine, Python environment, global PATH or CUDA installation was changed. Existing Scriberr settings/templates were used. Codex handled configuration and QA; no new source or recordings were sent to Claude or another external model.

## Daily use

The existing **Local Minutes** desktop shortcut now starts both the app and the isolated local Ollama server. Open a completed recording, open its action menu, select **AI Summary**, choose **Local Minutes - UNVERIFIED AI draft**, then **Generate Summary**. Reopen the same dialog to read the saved result. Copy/Download are existing upstream controls; automatic Obsidian meeting export is not implemented.

Use short transcripts (conservatively under about 1,000 spoken words) for this configuration and review every output against the transcript/audio. This is a manual working limit, not an enforced application limit or tested long-meeting capacity. Do not run transcription and summary generation simultaneously until concurrent GPU use is qualified. The model unloads after each request to release memory.

Do not treat an item as an accepted task merely because the model places it under Action items. No reminders, tasks or calendar events are created by this setup. Technical/code claims remain **UNVERIFIED**, including claims a speaker calls verified. A template is not a verification control.

## Installation and configuration

- Portable [Ollama v0.35.0](https://github.com/ollama/ollama/releases/tag/v0.35.0), MIT, from the official Windows AMD64 ZIP; its bundled GPU libraries and notices are retained. Archive SHA256: `d6f7d3dd4f5d013553a78c1e78b2521fcf41d43dd2863e4596cdc046fe6036db` (verified against official release metadata).
- Binary: sibling `baseline-runtime/tools/ollama-v0.35.0/ollama.exe`; models: `baseline-runtime/ollama-models`; private logs: `baseline-runtime/logs`. The normal installer was not run. Ollama may create its own identity files in the normal user `.ollama` directory; these are private and excluded from Git. No account/sign-in/API key was required.
- App provider: Ollama, `http://127.0.0.1:11434`. Both services bind loopback. Ollama has `OLLAMA_NO_CLOUD=1`; cloud rejection was tested. [Official local-only configuration](https://docs.ollama.com/faq#how-do-i-disable-ollama-cloud-features).
- Final provisional model: [Qwen2.5 7B Instruct Q4_K_M](https://ollama.com/library/qwen2.5:7b-instruct-q4_K_M), Apache-2.0, approximately 4.7 GB of weights. Alias `local-minutes-summary:7b`; alias digest `0cdd1d073c6a842f98b6a5b6830d368f26f9228c664230a731226a127f9577b9`. Selected for compatibility, speed and bounded memory, **not because it passed the accuracy gate**.
- Downloaded base-model digest: `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`. Record/check digests when reprovisioning: registry tags can change. No model refresh happens in the daily launcher.
- Alias settings: 8,192-token context, 2,048-token output limit, temperature 0.1, seed 42, explicit system instructions. See [LocalMinutesSummary.Modelfile](../scripts/windows/LocalMinutesSummary.Modelfile) and [saved template prompt](LOCAL_SUMMARY_PROMPT.txt). A seed does not guarantee identical output across versions/hardware.
- Process-only environment: OLLAMA_NOHISTORY=1, OLLAMA_DEBUG=0, OLLAMA_DEBUG_LOG_REQUESTS=0, OLLAMA_KEEP_ALIVE=0, OLLAMA_NUM_PARALLEL=1, OLLAMA_MAX_LOADED_MODELS=1, OLLAMA_FLASH_ATTENTION=1. No user/system environment variables or services/autostart entries were added.
- [Start-LocalOllama.ps1](../scripts/windows/Start-LocalOllama.ps1) validates executable ownership/loopback and reuses a healthy listener. It refuses a foreign listener. [Start-LocalMinutesWithSummaries.ps1](../scripts/windows/Start-LocalMinutesWithSummaries.ps1) starts it before the existing app launcher. Neither script downloads models. Default runtime path is relative to the checkout; binaries/models remain untracked.

Provisioning on another machine requires downloading/checking the official ZIP, extracting the same isolated layout, starting the local server, pulling the named base model, then `ollama create local-minutes-summary:7b -f LocalMinutesSummary.Modelfile` using the portable executable. Configure the existing UI provider/template as above. Inspect that machine first; do not reuse private credentials or paths from this laptop.

Once both runtimes are provisioned, `New-LocalMinutesShortcut.ps1 -WithSummaries` creates the combined shortcut. It preserves its existing refusal to overwrite a different shortcut. This laptop's previous shortcut was explicitly checked and backed up before changing its target; retain that backup for rollback. The original script without this flag creates the transcription-only shortcut on a fresh setup.

## Tests and model selection

Three small synthetic transcripts tested explicit commitments versus suggestions, no-action small talk, and a corrected deadline plus an embedded malicious instruction. Synthetic technical numbers/clauses are deliberately unverified test material, not code guidance. No private meeting recording was used.

| Check | Result | Evidence and limits |
|---|---|---|
| Portable startup / GPU inference | PASS | Actual generated outputs with CUDA; `/api/ps` reported the final model fully in GPU memory, 5,133,943,438 bytes, context 8192 |
| Cloud model disabled | PASS | A synthetic cloud-model request returned HTTP 403, `ollama cloud is disabled`; startup also logged cloud disabled |
| GPU memory released | PASS | `/api/ps` empty after requests with keep-alive zero |
| Local-only summary route | PASS | Scriberr persisted provider=ollama and loopback base URL; no cloud API credentials configured for this route |
| Normal UI generation / saving | PASS for mechanics | Two public samples generated and persisted with the selected local model; 1,063 and 1,247 characters respectively. Quality was not accepted |
| Public sample content quality | FAIL | Small-talk summary invented disagreement and misattributed quotes; aviation summary promoted requests/questions into action items |
| Fresh-process restart / duplicate launch | PASS | Both servers restarted; combined ready check 14.29s. Repeated launcher call reused both PIDs; both summaries and configuration remained in SQLite. Aviation summary reopened in the normal UI without regeneration |
| Existing installation/data preservation | PASS | All five pre-existing audio/transcript records, four dependency files, and three protected OpenWhispr file hashes unchanged |
| Actual Windows reboot | PARTIAL | Not performed; verify shortcut after next ordinary reboot |
| Physically disconnected network test | PARTIAL | Local inference and cloud rejection verified; the laptop was not disconnected and no system-wide firewall policy was installed |
| Synthetic no-action extraction | PASS, narrow | Final 7B run created no actions, but omitted the casual conversation from the executive summary |
| Synthetic action/speaker/quote accuracy | FAIL | Suggestions promoted to actions; one quote assigned to the wrong speaker; a corrected statement paraphrased inside quotation marks |
| Corrected deadline / injection rejection | PARTIAL | Tuesday retained instead of Monday or the injected Wednesday; no false certification issued, but an unapproved design suggestion was still listed as an action |
| UNVERIFIED prompt compliance | PARTIAL | Final selected runs displayed an unverified banner/claims section, but repeated technical claims were not consistently individually labelled. No enforced claim-verification state exists |
| Timestamp evidence links | NOT SUPPORTED in summary input | Upstream formatter sends speaker labels and text without segment times. Playback timestamps still work separately |
| Long transcript coverage / complete-output validation | NOT SUPPORTED | No summary chunking, input bound enforcement or reliable output validation; model limits can truncate or omit content |

Final 7B synthetic runs took 9.45s, 4.44s and 7.84s. These are three local checks, not a broad benchmark. Earlier 7B settings also failed quotation/attribution checks. Increasing system instructions improved the visible warning but did not fix semantic accuracy.

Comparison retained privately:

- Qwen3 8B Q4_K_M: 5.2 GB download, full GPU inference (~6.30 GB reported memory). Initial and stricter prompts both failed action/suggestion separation and evidence coverage. Revised runs took 17.03s / 13.59s / 22.23s.
- Qwen3.5 9B Q4_K_M: 6.6 GB download, full GPU inference (~5.73 GB reported memory). With 4,096 output tokens, two cases exhausted the budget in reasoning: one partial answer and one empty final answer. The no-action case completed. Scriberr does not send the API `think` control, and prompt `/no_think` did not solve this. This candidate is not selected. [Ollama thinking controls](https://docs.ollama.com/capabilities/thinking).
- All three model families used here have Apache-2.0 licenses. Their cached model manifests/licenses remain with the runtime; full model information and generated evidence are private. Unselected comparison models remain cached for reproducibility and are not loaded during daily startup. Aliases reuse weights rather than duplicate them.

## Current risks and next change

Items 1-4 are addressed in code by summary reliability increment 1 (pending Codex review and runtime verification); semantic accuracy is not. Item 5 is partly addressed: oversized input is now rejected and explicit limits are sent, but long meetings are unsupported.

1. **Accuracy gate failed:** do not call these outputs reliable meeting minutes or use generated actions automatically. The next implementation should produce structured candidates with segment IDs, then validate speaker/quote/time references against the actual transcript, distinguish proposals from accepted assignments, and require review before accepting actions. Validation must fail visibly; it cannot prove semantic truth by string matching alone.
2. **UNVERIFIED must be enforced outside the model:** store verification state, render a mandatory draft/unverified indicator, and require explicit authoritative-source verification. Do not rely on the model to print a warning consistently.
3. **Local logging privacy:** upstream `internal/llm/ollama.go` prints up to 2,000 characters of each streamed request, including transcript text, into app logs. Ollama debug request logging is off, but that does not stop Scriberr's logging. Logs remain local/private; remove this content logging before routinely processing confidential meetings.
4. **Completeness/error handling:** source review found that the summary handler can treat closed error channels/partial streams as completion, and the UI does not robustly distinguish all HTTP failures. No data loss from that channel issue was proven in this run. One public sample's missing final coverage sentence was reproduced identically from Ollama directly: it was model omission, not evidence of transport truncation. Add deterministic stream-completion/error tests before trusting the Ready label.
5. **Context mismatch:** model-advertised context can exceed configured runtime context, and the existing summary path provides no safe long-input handling. Add token budgeting/chunk coverage and output-limit detection before full meetings/conferences.

Preserve the existing transcription foundation. No UI redesign, Android work, external AI processing, automatic meeting export or broad refactor was done. Obsidian project-document snapshots are separate from future meeting export/LifeBot privacy controls.

Private evidence is under `baseline-runtime/summary-tests*`, `summary-app-evidence.json`, `summary-stream-comparison.json`, `ollama-model-inventory.json`, `openwhispr-after-summaries.json`, and the dated local logs. A consistent database backup preceded configuration: `data/backups/before-local-summaries-2026-10-02.db`. Existing audio/transcripts and dependency files were hash-checked; the original OpenWhispr executable, server and model hashes still match the pre-project inventory.

The existing Windows PowerShell 5.1 shortcut target was exercised with browser opening suppressed. Shortcut arguments were read back after updating; no full Windows reboot was performed. Repository launcher scripts parsed successfully and the original four launcher guard tests still passed. No application source changed, so no new Go/frontend build was needed.

Rollback: use the previous desktop shortcut saved at `baseline-runtime/data/backups/Local Minutes-before-summaries.lnk`, or invoke the original Start-LocalMinutes.ps1. Stop only the identified local Ollama server when idle; preserve model caches and app data. Turning off Ollama does not require changing Whisper. Do not restore the database backup over newer recordings merely to undo an LLM setting.
