# Acceptance tests

Use PASS, FAIL, PARTIAL or NOT SUPPORTED. For PARTIAL, explicitly identify unexecuted/blocked portions. Never treat source inspection as an end-to-end pass.

## Baseline qualification

1. Import a short non-sensitive WAV/M4A fixture; record provenance, duration and SHA-256. Verify both source and retained original hashes after processing.
2. Run WhisperX transcription with a documented model/revision, language, compute type and batch size. Inspect actual words against reference speech.
3. Prove CUDA: torch device probe plus inference logs/process telemetry. nvidia-smi alone is insufficient.
4. Diarize a short fixture with at least two known voices and several turns; report speaker count, correctly attributed turns, merges/splits and overlap limitations. Do not infer identities.
5. Check finite monotonic timestamps within audio duration; inspect word alignment and turn boundaries.
6. Play audio and seek from transcript timestamps; verify audible correspondence, not only HTTP success.
7. Generate a summary with a local LLM; record model and endpoint, compare decisions/actions against source, and identify hallucinations.
8. Save, close/restart and reopen; ensure transcript, speakers, notes, audio and summary persist.
9. With models provisioned, block external egress and repeat transcription and reopen. Distinguish offline inference from online first-run downloads.
10. Recheck existing Whisper/OpenWhispr binary/model hashes and environment inventory. Do not alter its settings or interrupt its process.

## Future product gates (not implemented in baseline)

- Immutable original audio across WAV/M4A/WebM import, conversion, retries, reprocessing and export.
- Portable package round-trip retains timestamps, markers, speakers, summaries and provenance without relying on the original database.
- Generic/custom marker types; personal configuration removed without changing core code.
- Technical claims default UNVERIFIED across UI, database and exports. Only explicit user verification with authoritative source evidence changes state; edits invalidate prior verification.
- Local-only mode prevents cloud-provider and webhook egress; no subscription/account required for core inference.
- Library search returns the correct session and audio timestamp; malformed packages fail safely.
- Obsidian export creates readable summary/marker/speaker transcript notes with stable IDs; long transcripts remain completely retrievable within the assistant's read limits.
- Re-export is idempotent, preserves user edits, detects conflicts and retries failed writes without duplicates or false success.
- Assistant retrieval answers a synthetic meeting question with the correct session/date/timestamp; conflicting sessions and missing evidence remain explicit. Technical claims stay UNVERIFIED unless independently verified.
- Test private-meeting exclusion before cloud-backed assistant access; metadata alone is not a privacy control. Verify phone/remote access and audio links separately from local Markdown export, including host-offline behavior.

## Local summary acceptance additions - 2026-10-02
- Require actual local GPU inference and a saved/reopened app result; these passed in LOCAL_SUMMARIES.md.
- Treat explicit commitments, unassigned suggestions, rejected proposals, corrected deadlines and embedded instructions as separate fixtures. Current models FAIL complete semantic acceptance; retain negative results.
- Exact evidence quotes must exist in the cited speaker/segment, and timestamp references must come from source data. Current prompts do not enforce this.
- Technical/regulatory state must remain UNVERIFIED in storage, display and export until explicit authoritative verification. Model-generated warnings alone do not pass.
- Oversized input, exhausted output budgets, incomplete streams and provider failures must produce visible incomplete/failed status, never a silently accepted summary. Not yet implemented.

## Summary reliability increment 1 - 2026-10-02 (branch `feature/summary-reliability-v1`)
Synthetic, mocked-provider coverage only; see docs/SUMMARY_RELIABILITY.md. These are source/unit/integration results, not runtime acceptance.
- Status truthfulness (provider error, unreachable, cancellation, stream without completion, unreadable stream lines, output limit, context overflow, empty, malformed, oversized input, no segments, interrupted): automated PASS; failed attempts never replace the saved summary or job cache. Windows runtime: NOT RUN.
- Evidence references: segment existence, quote-in-cited-segment, speaker and timestamps from the transcript, rejected items kept separately: automated PASS. Real-model output quality: NOT RUN.
- Semantic fixtures (suggestion vs commitment, refused purchase, Monday corrected to Tuesday, missing owner/deadline, wrong speaker, fabricated reference/quote, injection, no actions): flagged for review by lexical checks in automated tests. This is PARTIAL by design: lexical checks miss other phrasings and cannot detect omissions. Re-score with the real model.
- Technical claims UNVERIFIED in storage, API, UI and exported Markdown, including legacy summaries: automated PASS; no verification workflow exists.
- Summary-path logs free of transcript, prompt, model output and provider text (including SQL values): automated PASS. Runtime log inspection: NOT RUN.
