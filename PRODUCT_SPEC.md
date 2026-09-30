# Local Minutes

Status: baseline qualification only. Foundation: Scriberr. No product feature implementation authorized beyond baseline setup.

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

## Technical and regulatory claims

Every extracted building/plumbing/gas code, standard, clause, dimension, pressure or interpretation is **UNVERIFIED** by default. Transcription confidence, repeated statements, speaker credentials and AI agreement cannot verify a claim. Verification requires an explicit human action against an authoritative source, recording the source, edition/jurisdiction, relevant passage, verifier and date. Preserve the spoken wording separately from the claim and verification record. Changed claims require renewed verification. This rule must apply to storage, UI, summaries and exports, not just a prompt.

## Boundaries and development roles

Modular, hardware-independent where practical, Windows-friendly now, Android-friendly later, open-source friendly and maintainable. Claude is primary implementation; ChatGPT handles architecture review, QA and verification. Keep small commits and documented decisions.

No redesign, Android app, personal integration, engine replacement or broad refactor in this baseline. Stop after reporting baseline results and obtain user approval for feature work.
