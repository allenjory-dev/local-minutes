# Foundation decision — provisional Scriberr baseline

Date: 2026-09-30. Decision: retain Scriberr for the bounded desktop processing baseline. This is not production acceptance; actual pipeline tests control the next decision.

| Criterion | Scriberr | Meetily Community | Meetily-ActuallyFree |
|---|---|---|---|
| Local processing | Yes, local adapters | Yes | Yes |
| CUDA | WhisperX/PyTorch; Docker and configurable device | Source-build CUDA; packaged Windows documentation identifies Vulkan | Universal Windows installer selects CUDA/Vulkan/CPU |
| Transcription | WhisperX with alignment; Parakeet/Canary alternatives | Whisper.cpp/Parakeet | Whisper.cpp/Parakeet |
| Diarization | Pyannote and Sortformer | Do not assume marketing's Pro features are in Community | Bundled pyannote/WeSpeaker ONNX, optional Nemotron/Sortformer |
| Timed transcript | Yes | Yes | Yes, audio seek and speaker editing |
| Extensibility | Go interfaces + Python adapters + REST API | Tauri/Rust application | Tauri 2/Rust application; Next.js UI |
| Windows installation | Available binary, more ML setup risk | Native installer | Strongest packaged Windows path of this comparison |
| License | MIT; separate model terms | MIT Community; separate commercial edition | MIT, original Zackriya notices retained; model terms separate |
| Maintenance | September status update; preceding code April; release December 2025 | API reports push July 2026 | Active September 29; v0.2.18 released September 29 |
| Future Android ingestion | Existing HTTP upload/job API and folder watcher | Desktop command boundary | Desktop invoke/event boundary; architecture explicitly has no server |

Meetily-ActuallyFree is the strongest alternative and likely easier to install as a standalone Windows meeting recorder. It is not clearly superior for this project's immediate **imported audio -> desktop processing -> future mobile integration** foundation: Scriberr already exposes the service/API and interchangeable Python engines wanted here. Do not silently switch foundations if Scriberr's baseline fails; present the evidence and reassess.

Meetily's public product page places speaker diarization and API/MCP access in Pro. Community source and marketing must be distinguished; no subscription is acceptable for Local Minutes core.

Also screened whisper.cpp: actively maintained MIT engine with CUDA/Windows support, but it is an engine rather than the complete library/notes/summaries/API product foundation needed. Adopting it directly would require more application work.

## Scriberr forks

Bounded check of eight most-starred and twelve newest forks through GitHub API. Prominent paulirish/Scriberr was **0 ahead / 43 behind** current upstream in the comparison response. Recent push dates alone do not demonstrate improvements (many forks inherit upstream timestamps). No inspected fork demonstrated a clearly better, maintained complete technical base. This is not an exhaustive review of all 266 forks.

## References

- https://github.com/rishikanthc/Scriberr
- https://github.com/zackriya-meetily/meetily
- https://meetily.ai/downloads/
- https://meetily.ai/mcp
- https://github.com/TylerBuza/Meetily-ActuallyFree
- https://github.com/TylerBuza/Meetily-ActuallyFree/blob/main/ARCHITECTURE.md
- https://github.com/TylerBuza/Meetily-ActuallyFree/blob/main/LICENSE.md
- https://github.com/TylerBuza/Meetily-ActuallyFree/releases/tag/v0.2.18
- https://github.com/ggml-org/whisper.cpp
