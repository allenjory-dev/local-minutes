# Windows diarization qualification - 2026-10-02

Model: pyannote/speaker-diarization-community-1 (CC-BY-4.0). User's Hugging Face model-access approval was confirmed in the browser. No paid cloud inference service is used. Speaker qualification now passes on a short public fixture; see Qualification results below for scope and accuracy limits.

## Isolated runtime corrections

The original pyannote environment selected CPU wheels because Windows reports AMD64, while its CUDA wheel marker only recognized Linux x86_64. The embedded project now recognizes Windows AMD64/x86_64 for the configured PyTorch CUDA wheel index, retaining CPU selection for macOS and other architectures. Torch and torchaudio are pinned together at 2.8.0; TorchCodec is constrained to the compatible 0.7 series. The previous unconstrained resolution paired torch 2.8 with incompatible TorchCodec 0.17.

Only the dedicated pyannote environment was synchronized. Its configuration/lock were backed up first; three packages changed from cached wheels: torch/torchaudio CPU -> cu126, TorchCodec 0.17 -> 0.7. WhisperX and existing global/OpenWhispr environments were not modified. A CUDA tensor operation now passes in pyannote's environment on this laptop.

Native pyannote filepath decoding still failed with missing TorchCodec/FFmpeg shared DLL dependencies. The embedded script now uses the existing FFmpeg executable to decode a read-only source to mono 16 kHz float32 audio in memory, then uses pyannote's documented waveform input. It does not replace system FFmpeg, write to source audio, or install system DLLs. Whole-recording waveform memory use scales with duration; very long recordings still need separate qualification.

## Evidence and limits

- PASS: actual CUDA tensor computation in the dedicated environment.
- PASS: wheel-marker tests for Windows AMD64/x86_64, Linux x86_64/aarch64 and macOS arm64/x86_64.
- PASS: real FFmpeg stereo 48 kHz -> mono 16 kHz decoding, float32 shape/finite checks and unchanged source hash; invalid source fails without altering bytes.
- Initially pending model/app tests are now completed below. CUDA and decoder checks alone were not used as speaker-quality proof.
- The pyannote import may still warn that native TorchCodec decoding is unavailable. The explicit waveform path is the intended route for this installation.

## Credentials

The user creates the token in their own browser. A separate local masked dialog can save it using Windows DPAPI under the user's Windows account in the ignored runtime secrets directory. The agent must not read or print its plaintext. Never put a real token into chat, Git, Obsidian, command arguments or Scriberr's persisted token/profile fields. Use the patched adapter's child environment when qualification resumes. Existing application-wide credential storage has not been redesigned.

## Primary references

- Model access, license, local GPU and waveform/offline use: https://huggingface.co/pyannote/speaker-diarization-community-1
- Token scopes: https://huggingface.co/docs/hub/security-tokens
- TorchCodec compatibility and Windows shared FFmpeg requirements: https://github.com/meta-pytorch/torchcodec

## App selection correction

The first app-level WhisperX + Pyannote run failed because the UI alias `pyannote` was mapped to the legacy gated `speaker-diarization-3.1` model. Community-1 access does not imply access to 3.1. The adapter now maps the generic alias and its default to Community-1, matching the dedicated adapter and installed WhisperX default. Explicit legacy 3.1 selections remain unchanged; Community-1 is an allowed explicit choice. No transcription engine or Python packages were replaced. A regression test reproduced the default/alias/validation failures before the fix; all four model-selection cases and affected transcription/API tests pass afterward. End-to-end retry is recorded separately.

## Qualification results - 2026-10-02

- PASS: Community-1 download into the isolated Hugging Face cache; model revision `3533c8cf8e369892e6b79ff1bf80f7b0286a54ee`. Credentials saved locally with Windows DPAPI; no real token supplied in chat or command arguments.
- PASS: dedicated Pyannote adapter used CUDA and automatically found two speakers across 11 turns in a public 30-second fixture. No min/max speaker constraint. About 15.6 seconds including first model acquisition/loading.
- PASS: cached dedicated diarization under network restriction with HF/Transformers/uv offline flags and a synthetic token produced identical segments.
- Reference comparison: 5.99% diarization error rate, zero collar, overlap included, optimal label mapping. Of 24.35 reference speaker-seconds: 23.881 correct, 0.0206 confused, 0.4484 missed; 0.9890 false-alarm speaker-seconds. This is one short fixture, not a real-meeting accuracy guarantee.
- PASS: normal browser import/start -> WhisperX tiny/CUDA/float16/batch 2 -> Community-1 -> saved transcript. Final run about 10.4 seconds; 14 transcript segments, 78 timed words, two speaker labels, every word labelled and timestamps bounded to the source. Final app token field was explicitly cleared; database token field is empty. Uploaded original hash matches source and original-audio reference is retained.
- PASS: combined WhisperX transcription/alignment/diarization ran with cached models, synthetic credential and HF/Transformers/uv offline flags in the network-restricted environment. Tiny/en/Silero/CUDA/float16/batch 2; 14 segments, 78 timed and speaker-labelled words. This qualifies the engine pipeline, not every app startup/network behavior.
- LIMIT: transcript merging can assign brief or overlapping speech incorrectly (the initial short greeting differs from the reference); the standalone diarization error rate must not be reported as end-to-end transcript accuracy. Tiny-model recognition contains wording errors; larger model and real meeting tests remain necessary.
- LIMIT: upstream output reports confidence 1.0 for diarization segments as a constant. It is not calibrated confidence and cannot verify identity or technical claims.

Runtime evidence is private, outside Git: diarization-smoke/quality.json, result-online.json, logs/diarization-offline-run.log, logs/combined-offline-run.log and app-diarization-evidence.json. Original application, earlier safety binaries, and pre-diarization SQLite backup are retained. No global Python/CUDA/Whisper modifications.

