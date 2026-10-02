# Upstream provenance

- Project: Scriberr
- Canonical URL: https://github.com/rishikanthc/Scriberr
- License: MIT, copyright (c) 2025 Scriberr; full original LICENSE retained unchanged.
- Original fork commit: `a353078fd96b8aca4002681813524b7397c90df1`
- Fork date: 2026-09-30
- User fork: https://github.com/allenjory-dev/local-minutes
- origin: user's repository; upstream: original project, with local push URL disabled.
- Development branch: develop. main remains the upstream baseline.

## Change log

2026-10-02: local summary configuration and qualification using unchanged upstream application code. Added portable Ollama startup wrapper, reproducible model definition, saved prompt and test report. Existing settings connect to loopback with cloud disabled. Desktop shortcut now starts both local services. Local generation/persistence pass; semantic accuracy and enforced claim-verification gates do not. No summary extraction feature, original audio or transcription dependency changes. See LOCAL_SUMMARIES.md.

2026-10-02: opt-in startup model selection. `SCRIBERR_STARTUP_MODELS` is an optional comma-separated list of registered model IDs; when set, only those adapters are prepared, the whole list is validated before any preparation runs, and unknown IDs or preparation failures return an error instead of a warning. Unset configuration keeps upstream's prepare-every-adapter behavior, including its tolerance of preparation failures. Selecting `whisperx` alone leaves the separate pyannote environment unprepared because WhisperX diarizes internally. Added registry and service regression tests. No engine, default or UI change.

2026-10-02: diarization qualification correction for Windows AMD64 wheel selection, matched torch/torchaudio 2.8 and TorchCodec 0.7 pins, and read-only FFmpeg waveform decoding to avoid missing shared-DLL dependencies. Added platform-marker and real-decoder tests. Model inference still requires separate proof; see DIARIZATION_SETUP.md.

2026-10-02: user-approved safety follow-up. Audio uploads retain WebM originals with a separate playback derivative and an additive original_audio_path reference. WhisperX/pyannote tokens move from process arguments to child environments; child log output redacts the resolved token across write boundaries. Added focused regression tests. See SAFETY_PATCH.md for scope, remaining ingestion/storage risks and verification.

2026-09-30: add Local Minutes product requirements, developer rules, acceptance criteria, environment/architecture assessment, foundation decision and roadmap. No application feature changes, attribution removal or transcription-engine replacement.

2026-09-30: add build-versus-reuse judgment, setup/handoff notes and observed native setup risks. Baseline binary built from the original upstream application revision. Local runtime uses WhisperX commit `771b4a14a9486f8fd5aef18ef49e35d639523dd3` as fetched by upstream initialization; its own license is BSD-2-Clause and remains in its runtime checkout.

Runtime toolchains, models, user data, machine inventory and secrets are excluded from public Git history. Baseline runtime dependencies and results are recorded separately. Model licenses and access conditions must be tracked independently from the application's MIT license.

2026-09-30: record the user's Obsidian/personal-assistant requirement, proposed export contract and acceptance gates. No export feature or LifeBot code changes. A dated project-document snapshot is saved separately to the user's configured local vault.

2026-10-02: correct WhisperX's generic Pyannote selection/default to Community-1, matching the dedicated adapter and installed WhisperX. Preserve explicit legacy 3.1 choices; add selection/validation regression tests. The mismatch was reproduced by an app-level gated-model failure.
