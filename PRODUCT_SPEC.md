# Local Minutes

Status: baseline qualification plus scoped safety and operational follow-ups approved on 2026-10-02. Foundation: Scriberr. Desktop startup and saved GPU profile are verified. The user then authorized local summary configuration/qualification: local GPU draft generation and saving work, but accuracy/claim-verification acceptance failed. See docs/LOCAL_SUMMARIES.md. Do not treat generated actions as accepted tasks or prompt warnings as enforced verification. A scoped summary reliability increment (truthful status, evidence-linked candidates, enforced UNVERIFIED state, transcript-free logs) is on `feature/summary-reliability-v1` pending review; see docs/SUMMARY_RELIABILITY.md. Session packages, automatic Obsidian export, Android and additional application changes remain deferred.

## Purpose

Private, local-first, subscription-free meeting recording and intelligence built on an existing open-source foundation. Core transcription, diarization and local AI must work without paid cloud APIs once dependencies and models are provisioned. Optional cloud integrations must be explicit and never required.

Immediate goal: prove short imported audio -> transcript -> anonymous speakers -> timestamps -> usable persistent output on Windows, preferably with NVIDIA acceleration. Preserve the user's existing Whisper installation and global environments.

## Intended workflow

Future: RODE Wireless GO Gen 3 -> Samsung S26 Ultra capture -> portable session/audio package -> laptop -> Whisper/WhisperX or an explicitly approved engine -> diarization -> local AI -> searchable library. Android development is deferred.

Session profiles: Meeting, Conference/Summit, Personal Note, and custom profiles. Speakers use anonymous stable labels (Speaker 1, Speaker 2, etc.); optional user renaming must not imply verified identity.

## Portable sessions — proposed contract, not implemented

A versioned directory package should contain session.json, untouched audio.wav or audio.m4a, markers.json, transcript.json and summary.md. Original audio bytes are immutable. Store hashes, source metadata, durations, engine/model revisions and processing history. Resampled/normalized audio belongs in derivatives. Reprocessing must preserve originals and prior provenance. A database may index sessions but must not be the sole usable copy.

Markers use configurable type IDs, labels, timestamps on the original audio timeline and optional text. Generic defaults: Important, Decision, Action, Question, Follow Up, Idea, Custom. Personal types such as Done By the Book or Verify Code belong only in optional user configuration. No special-case product logic for these names.

AI outputs: executive summary, major takeaways, decisions, action items, follow-ups, important moments, questions and ideas, with links to transcript/audio evidence. Prefer local Ollama where practical. Do not invent commitments, deadlines, owners or missing speech.

## Obsidian and personal-assistant access

Added 2026-09-30 at the user's request: completed meeting/conference records must be exportable to a configured Obsidian vault so a personal assistant such as LifeBot can retrieve what happened later, including while the user is away from the laptop. This is a required future capability, not an implemented baseline feature.

Export readable Markdown: session title/date/type, summary, takeaways, decisions, actions, follow-ups, questions, ideas, markers, speaker-labelled transcript and source timestamps. Include stable session IDs, processing/export revisions, source-audio hashes and links to the portable session package. Keep long transcripts in linked, bounded-size parts so assistant read limits cannot silently hide later content. Preserve UNVERIFIED status and explicit verification evidence in every exported view.

The Obsidian adapter is optional and generic: vault path, destination folders and audio-copy policy are configuration. LifeBot and the user's vault paths must not be hard-coded into the core. Text must remain usable without Local Minutes running. Keep original audio in the portable package; optionally copy/link it into the vault without altering it. Audio links need a device-aware strategy; a Windows path or localhost URL is not a working phone playback link.

Repeated exports must update the same session without duplicates, preserve user-authored notes and flag conflicts. Failed exports must remain visible and retryable. Obsidian export must not automatically create tasks, reminders or calendar entries from extracted actions.

Saving locally and sharing with a cloud-backed assistant are separate choices. The current personal assistant may use external models; export alone must not be described as fully local Q&A. Before enabling automatic export of private meetings to an assistant-readable vault, verify an enforceable local-only/exclusion or explicitly approved cloud-processing path. Metadata labels alone cannot enforce privacy. Remote access also depends on the assistant host being awake/reachable or a separately approved hosting/sync arrangement.

See docs/OBSIDIAN_INTEGRATION.md for the proposed contract and validation gates. Integration implementation remains deferred until baseline approval.

## Technical and regulatory claims

Every extracted building/plumbing/gas code, standard, clause, dimension, pressure or interpretation is **UNVERIFIED** by default. Transcription confidence, repeated statements, speaker credentials and AI agreement cannot verify a claim. Verification requires an explicit human action against an authoritative source, recording the source, edition/jurisdiction, relevant passage, verifier and date. Preserve the spoken wording separately from the claim and verification record. Changed claims require renewed verification. This rule must apply to storage, UI, summaries and exports, not just a prompt.

## Boundaries and development roles

Modular, hardware-independent where practical, Windows-friendly now, Android-friendly later, open-source friendly and maintainable. Claude is primary implementation; ChatGPT handles architecture review, QA and verification. Keep small commits and documented decisions.

No redesign, Android app, personal integration, engine replacement or broad refactor in this baseline. Stop after reporting baseline results and obtain user approval for feature work.
