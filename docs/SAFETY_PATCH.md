# First safety follow-up - 2026-10-02

The user approved the proposed original-audio and token-logging fixes, followed by further diarization qualification. This is a small correction to the existing architecture, not a new engine, UI redesign or completed portable-session implementation.

## Audio upload

The audio upload endpoint now retains WebM bytes and writes normalized playback audio to a separate `<upload-id>.playback.mp3`. The job keeps its original ID, existing `audio_path` still supplies playback/processing, and the additive `original_audio_path` field records the retained source. Other audio uploads point both fields at the same source. SQLite auto-migration adds the nullable column; legacy rows remain compatible and are not backfilled with invented provenance.

Conversion and database-creation failures retain the original and remove only the failed/new derivative. Failed uploads can therefore leave an unindexed source file in the upload directory; the warning log records its path for recovery. Automatic cleanup is deliberately not added. Sources deleted by earlier versions cannot be restored by this patch.

Scope: `UploadAudio`, used by normal audio import/browser audio uploads. Video extraction, YouTube ingestion, multitrack ingestion and explicit session deletion remain separate upstream paths, not certified by these tests. The existing delete endpoint still deletes its `audio_path`; for converted WebM the separately retained original remains. A consistent source-retention/deletion policy and recovery UI require a later change. Do not describe this patch as complete product-wide immutability.

## Hugging Face credentials

WhisperX and pyannote now receive the resolved token through their child `HF_TOKEN` environment, not command-line arguments. This removes it from the application's command log and the child process command line. The embedded pyannote script accepts that environment value; WhisperX/Hugging Face supports it already.

The same resolved token is redacted from child stdout/stderr before writing transcription logs, including tokens split across writes. Error-tail responses read that sanitized file. Credential-free diagnostics remain intact. No real credential is used in tests.

This is not a complete credential-storage redesign. Existing token fields in requests/profiles/job/execution data can still persist user-entered values; old logs are not rewritten. For qualification, use a process-local token supplied securely by the user and leave token fields in the application empty. Environment variables are still accessible to sufficiently privileged local processes. No claim is made that this defeats an administrator or hostile code on the laptop.

## Verification

- PASS: six upload integration cases using isolated temporary directories and SQLite: WAV, WebM, uppercase WebM, conversion failure, WAV database failure and WebM database failure. Actual ffmpeg creates/converts a short synthetic fixture. Source hashes, persisted references and derivative existence/cleanup are checked. WebM cases require ffmpeg in the test process PATH (they were executed, not skipped, on this laptop).
- PASS: credential redaction at every log chunk size, repeated tokens, ordinary logs, final partial text and destination write errors; child environment isolation; both adapters' command arguments exclude a synthetic token and token flags.
- Full `go test ./...`: existing failures remain in `TestDatabaseTestSuite/TestUserCRUD` and `TestLLMTestSuite/TestGetModelsTimeout`, matching the baseline. Internal API/transcription/adapter tests pass. Tests were not weakened to obtain a green suite.
- Runtime upgrade and post-upgrade smoke evidence are recorded separately in private local results. Unit/integration checks alone do not prove diarization or speaker quality.

## Model access

The intended model is `pyannote/speaker-diarization-community-1`, under CC-BY-4.0, separate from Scriberr's MIT license. Its Hugging Face gate requires the user's agreement to share contact details. The model card documents local processing and offline use after provisioning: https://huggingface.co/pyannote/speaker-diarization-community-1. No model files are redistributed by this patch. User acceptance and a securely supplied download token remain prerequisites.
