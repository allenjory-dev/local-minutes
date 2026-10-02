# Windows diarization qualification - 2026-10-02

Model: pyannote/speaker-diarization-community-1 (CC-BY-4.0). User's Hugging Face model-access approval was confirmed in the browser. No paid cloud inference service is used. A download token and a completed speaker test are still required before marking diarization PASS.

## Isolated runtime corrections

The original pyannote environment selected CPU wheels because Windows reports AMD64, while its CUDA wheel marker only recognized Linux x86_64. The embedded project now recognizes Windows AMD64/x86_64 for the configured PyTorch CUDA wheel index, retaining CPU selection for macOS and other architectures. Torch and torchaudio are pinned together at 2.8.0; TorchCodec is constrained to the compatible 0.7 series. The previous unconstrained resolution paired torch 2.8 with incompatible TorchCodec 0.17.

Only the dedicated pyannote environment was synchronized. Its configuration/lock were backed up first; three packages changed from cached wheels: torch/torchaudio CPU -> cu126, TorchCodec 0.17 -> 0.7. WhisperX and existing global/OpenWhispr environments were not modified. A CUDA tensor operation now passes in pyannote's environment on this laptop.

Native pyannote filepath decoding still failed with missing TorchCodec/FFmpeg shared DLL dependencies. The embedded script now uses the existing FFmpeg executable to decode a read-only source to mono 16 kHz float32 audio in memory, then uses pyannote's documented waveform input. It does not replace system FFmpeg, write to source audio, or install system DLLs. Whole-recording waveform memory use scales with duration; very long recordings still need separate qualification.

## Evidence and limits

- PASS: actual CUDA tensor computation in the dedicated environment.
- PASS: wheel-marker tests for Windows AMD64/x86_64, Linux x86_64/aarch64 and macOS arm64/x86_64.
- PASS: real FFmpeg stereo 48 kHz -> mono 16 kHz decoding, float32 shape/finite checks and unchanged source hash; invalid source fails without altering bytes.
- Pending: authenticated model download, actual diarization, speaker-count/turn attribution assessment, app integration and cached offline diarization. Do not infer speaker quality from CUDA or decoder tests.
- The pyannote import may still warn that native TorchCodec decoding is unavailable. The explicit waveform path is the intended route for this installation.

## Credentials

The user creates the token in their own browser. A separate local masked dialog can save it using Windows DPAPI under the user's Windows account in the ignored runtime secrets directory. The agent must not read or print its plaintext. Never put a real token into chat, Git, Obsidian, command arguments or Scriberr's persisted token/profile fields. Use the patched adapter's child environment when qualification resumes. Existing application-wide credential storage has not been redesigned.

## Primary references

- Model access, license, local GPU and waveform/offline use: https://huggingface.co/pyannote/speaker-diarization-community-1
- Token scopes: https://huggingface.co/docs/hub/security-tokens
- TorchCodec compatibility and Windows shared FFmpeg requirements: https://github.com/meta-pytorch/torchcodec
