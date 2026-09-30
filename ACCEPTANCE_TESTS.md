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
