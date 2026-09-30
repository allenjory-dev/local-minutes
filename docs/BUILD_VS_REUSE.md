# Build from scratch or reuse?

Assessment date: 2026-09-30. Recommendation: reuse an existing foundation, with a bounded validation gate; do not build the complete application from scratch now.

## Why reuse wins at this stage

Local Minutes needs mature, largely standard capabilities: audio import, queued inference, transcript alignment, speaker segmentation, waveform playback, authentication, persistence, notes and local LLM integration. Scriberr already connects these pieces. Rebuilding them would create a substantial new testing and maintenance burden before proving the user's distinctive requirements: portable sessions, configurable markers and trustworthy handling of unverified technical claims.

Even a from-scratch application would still depend on WhisperX/faster-whisper, pyannote or equivalent engines, GPU libraries, audio codecs and local LLM runtimes. It would not eliminate model-access conditions or CUDA compatibility work.

## Reuse is conditional, not blind adoption

Scriberr's current integration has real costs: mutable runtime dependencies, broad model initialization, a long maintenance pause, deletion of imported WebM originals, credential-bearing argument logging and Windows qualification gaps. A successful HTTP startup is not enough. Baseline tests determine whether to continue.

Meetily-ActuallyFree is the principal fallback when native Windows installation and integrated diarization outweigh Scriberr's ready-made HTTP processing service. It has active releases, bundled runtime support and useful speaker features. Its Tauri/Rust architecture would require an additional service/import boundary for the planned Android-to-laptop workflow. No clear reason has yet emerged to discard both projects and rebuild everything.

## Decision gate after baseline

- Continue with Scriberr if short local inference, speakers, timestamps, playback and persistence can be made repeatable through small, understandable setup/correction work.
- Reassess Meetily-ActuallyFree if native model setup remains fragile or requires substantial engine/platform rewrites.
- Consider a new thin application only if both foundations impose greater architectural change than retaining their useful components. Reuse proven ML/audio libraries even then.

Do not interpret this recommendation as authorization to fix or redesign the application. The current stage ends with evidence and a user decision. See FOUNDATION_DECISION.md for comparison sources and BASELINE_TEST_RESULTS.md for local execution results.
