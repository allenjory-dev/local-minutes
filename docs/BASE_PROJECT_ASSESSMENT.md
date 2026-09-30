# Scriberr foundation assessment

Verified 2026-09-30 against https://github.com/rishikanthc/Scriberr at `a353078fd96b8aca4002681813524b7397c90df1`.

## Identity, license and maintenance

The supplied repository is current, public, not archived, default branch main. Its LICENSE is MIT, copyright (c) 2025 Scriberr. Modification, forking and redistribution are expressly permitted subject to retaining the copyright and permission notice. Preserve the full LICENSE. Model weights and dependencies have separate terms; MIT does not automatically license everything downloaded at runtime.

Latest release observed: v1.2.0, published 2025-12-17, including Windows x86_64/arm64 archives. Current HEAD is a 2026-09-20 status update announcing resumed development; immediately preceding source changes are from 2026-04-21. There was a substantial development pause. Current HEAD and the release are not interchangeable baselines.

## Architecture and directory map

| Area | Implementation |
|---|---|
| Backend | Go 1.24, Gin HTTP routing, GORM, pure-Go SQLite; cmd/server/main.go |
| UI | web/frontend: React 19, TypeScript 5.8, Vite 7, Tailwind 4, Radix, TanStack Query/Table, Zustand, WaveSurfer; PWA |
| Embedded UI | internal/web/static.go embeds generated dist; build frontend before compiling server |
| Processing | internal/transcription: unified orchestration, registry, adapter interfaces, queue integration, preprocessing and result merging |
| Model workers | Python subprocesses managed by uv; separate WhisperX, pyannote, NeMo and Voxtral environments |
| Storage | SQLite jobs, profiles, notes, summaries, auth, speaker mappings; uploads and transcript artifacts on filesystem |
| API | internal/api/router.go; /api/v1, JWT/API-key authentication, upload/start/status/transcript/audio, notes, speakers, summaries, chat, profiles; SSE progress |
| Other integration | cmd/scriberr-cli, watched folders, optional callback webhooks |
| Tests | tests/*.go; internal queue/transcription/webhook/SSE tests; Python adapter tests; Ubuntu CI builds/lints frontend and runs Go tests |

## Supported features: source evidence, not runtime passes

| Requested capability | Evidence and limits |
|---|---|
| Whisper / WhisperX | whisperx_adapter.go clones m-bain/WhisperX and invokes python -m whisperx through uv. Configurable model/device/compute type/batch size. |
| Speaker diarization | WhisperX integrated diarization; standalone PyAnnote and NVIDIA Sortformer adapters; overlap-based merging onto transcript words/segments. Speaker rename mappings supported. |
| pyannote | Dedicated pyannote.audio 4.0.2 environment; Hugging Face token explicitly required by standalone adapter. WhisperX also supports pyannote diarization. Model-access acceptance needed. |
| NVIDIA CUDA | CUDA device configuration; CUDA 12.6/cu126 Docker image documented for RTX 40 series; separate Blackwell image. Host runtime must be tested. |
| Timestamp playback | Segment/word timings, WaveSurfer playback, transcript seek/follow-along. |
| Summaries / local LLM | internal/llm/ollama.go and summary templates. Local Ollama supported; optional OpenAI-compatible providers and cloud transcription also exist. Local-only is a configuration choice, not a network enforcement policy. |
| Notes/highlights | Selection-based notes store word indices, start/end seconds, quoted text, markdown content. Not equivalent to configurable recording-time markers. |
| API | Versioned HTTP API with upload and job operations; useful desktop endpoint for later Android package ingestion. |
| Search | UI/library capabilities need runtime qualification; no verified cross-library full-text search guarantee from this inspection. |

## Installation and Windows

README documents Homebrew (macOS/Linux), manual binaries, CPU Docker and NVIDIA Docker. Docker isolates Linux ML dependencies well, but this laptop currently has neither Docker nor WSL. Docker Desktop GPU support requires the WSL2 backend: https://docs.docker.com/desktop/features/gpu/.

Windows builds are explicitly present in .goreleaser.yaml and release assets. Native Go + uv is worth a bounded baseline attempt without changing the system Python or enabling OS features. Windows support for every model dependency is not established by the existence of a Windows server binary. NeMo and platform-specific wheels are particular risks. Current source builds need Go, Node, and frontend assets; uv handles Python packages. ffmpeg is already installed.

## Architectural risks requiring follow-up

1. **Original-audio retention violation:** internal/api/handlers.go UploadAudio converts uploaded WebM to normalized MP3 and removes the uploaded WebM. Local Minutes must eventually retain the original bytes and treat conversions as derivatives. Do not use sole copies or real recordings for baseline.
2. **Credential logging risk:** WhisperX and standalone PyAnnote adapters log joined command arguments; arguments can contain hf_token. Do not provide a real token to this unmodified baseline until the logging path is addressed or securely isolated. No token was supplied during assessment.
3. **Reproducibility:** startup clones mutable WhisperX main and rewrites dependency strings; it does not pin the checkout. Current WhisperX can outgrow those assumptions. uv sync includes extras/dev dependencies.
4. **Heavy startup:** all registered model environments initialize concurrently, including unused models; shared NeMo environment initialization can contend. Adapter errors are collected and logged without preventing overall readiness. A healthy HTTP endpoint does not prove transcription works.
5. **Offline claim needs qualification:** first-time packages/model downloads need internet. Cached use must be tested without egress, including restart. Optional cloud providers/webhooks must remain unused.
6. **Storage portability:** SQLite plus internal files is not the proposed versioned session package. Export/reimport, immutable originals, hashes, markers and provenance remain work.
7. **Regulatory verification absent:** spoken technical claims and LLM summaries are not authoritative. No verified/unverified architecture was found. This must be added before technical reliance.
8. **GPU contention:** two queue workers plus future local LLM can exceed laptop VRAM; benchmark small batches and sequential workloads first.
9. **Coverage limits:** no measured coverage percentage; no frontend test script in package.json; CI is Linux and is not proof of Windows/CUDA/real multi-speaker behavior. One transcription test file has .old suffix and is not active Go coverage.
10. **Security defaults:** default bind is 0.0.0.0. Use 127.0.0.1. HTTP localhost needs SECURE_COOKIES=false while retaining APP_ENV=production. User data and credentials must be excluded from Git.

Primary references: [source README](https://github.com/rishikanthc/Scriberr), [license](https://github.com/rishikanthc/Scriberr/blob/a353078fd96b8aca4002681813524b7397c90df1/LICENSE), [release](https://github.com/rishikanthc/Scriberr/releases/tag/v1.2.0). Source paths above refer to the pinned checkout.
