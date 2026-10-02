# Two-person recording qualification

Authorized on 2026-10-02 after the short public-fixture baseline passed. Scope: prepare a larger existing Whisper model and test one short real recording. No UI redesign, Android implementation, summarizer installation or automatic vault export.

## Recording to supply locally

Record 60-90 seconds with two people who know they are being recorded. A normal phone recorder is sufficient; RODE microphones are optional. Save/copy the original WAV or M4A to the laptop and provide its full path. No audio upload to chat or cloud inference is required.

Take 4-6 turns each. Include a few ordinary names, numbers, a fictional deadline/action and one brief natural interruption. Begin with each person identifying themselves as Speaker 1 or Speaker 2 so attribution can be checked. Avoid private details for this first test. Written expected wording or important phrases improves verification; do not claim word-error accuracy without a checked reference.

## Processing and evidence

1. Hash and inspect the original file's duration, codec, channels and sample rate. Keep the original untouched; preserve prior test sessions.
2. Use the existing isolated WhisperX engine with model `medium`, CUDA, float16, batch 2, alignment enabled and Community-1 diarization. Leave the app's Hugging Face token box empty; its launcher supplies the local credential. Start with automatic speaker count.
3. Record actual elapsed processing time and whether GPU execution succeeds. Stop on unexpected duration, memory failure or a request for unrelated model downloads; do not silently modify global dependencies.
4. Check the transcript against the audio/reference: omissions, invented words, names, numbers and technical statements. Technical/regulatory claims remain UNVERIFIED regardless of transcription accuracy.
5. Check two speaker labels and turn changes; explicitly inspect overlap/interruption and short utterances. Labels are arbitrary until mapped against the recording. A supplied speaker count is a separate constrained test and must not be confused with automatic detection.
6. Check word timestamps, audio playback/Listen navigation, saving/reopening and original hash. Record PASS, FAIL or PARTIAL with limits.
7. Keep recording, transcript and runtime evidence local and out of public Git. Save only the non-sensitive qualification report to the project documentation/Obsidian. Automatic export of private meeting content or cloud-backed LifeBot processing requires the separate privacy design already specified.

The existing tiny-model public-fixture result is a functional baseline, not a real-world accuracy guarantee. Compare tiny and medium on identical audio/settings when necessary to measure improvement; model size alone does not establish quality.

## Model provenance

Medium uses the existing faster-whisper CTranslate2 model path, not a replacement engine. Source: [Systran/faster-whisper-medium](https://huggingface.co/Systran/faster-whisper-medium), MIT; converted from OpenAI Whisper medium. Inspected revision: `08e178d48790749d25932bbc082711ddcfdfbc4f`. Selected runtime/model-card files total 1,530,573,740 bytes. Model card is retained in the isolated cache; model weights are not committed or copied to Obsidian.

Provisioning uses the current WhisperX virtual environment and its existing libraries. No pip/uv dependency update, global Python/CUDA/PATH change or modification to existing OpenWhispr is needed.

## Status

The user could not supply a personal recording yet and requested a public substitute. The public human-conversation test below is completed; personal phone/RODE/room capture remains untested.

## Public conversation results - 2026-10-02

Source: [AppTek Call-Center Dialogues](https://huggingface.co/datasets/apptek-com/apptek_callcenter_dialogues), Canadian English, human role-played agent/customer conversation, supplied manual transcript and speaker-labelled segments, CC-BY-SA-4.0. Dataset revision `b98967d9946f7f59f58d08624a2a00fe98fe0219`. Selected the lexicographically first Canadian recording (`en_CA_Agriculture_1586885.wav`) before inference, not the best result. Preserved the complete downloaded original and created a separate 84.29-second mono 16 kHz excerpt ending at a clean turn boundary. Only that excerpt was processed. No audio/transcripts are redistributed in this repository.

| Test | Result |
|---|---|
| Medium model provisioning | PASS: 1.53 GB, isolated cache; no dependency upgrade |
| Medium preflight on earlier 30s fixture | PASS: CUDA/offline, 12 segments/80 timed words, two speakers, 15.2s |
| Public excerpt, tiny engine run | PASS: CUDA/offline, two speakers, 140 timed words, 14.4s |
| Public excerpt, medium engine run | PASS: CUDA/offline, two speakers, 135 timed words, 15.0s |
| Public excerpt, normal app medium run | PASS: CUDA, two speakers, 18 segments/137 timed words, 15.0s |
| Audio/source preservation | PASS: original downloaded audio and uploaded excerpt hashes verified |
| Playback/timestamp navigation | PASS: Listen sought exactly to selected word start 28.909s; playback advanced to 75.261s and paused without error |
| Saving/reopening | PASS: completed transcript persisted and reopened |
| User's phone, microphones, room noise | PARTIAL: public substitute does not test the user's capture hardware |

### Measured comparison and limits

The controlled tiny/medium comparison uses identical CUDA/float16/batch 2, English, Silero VAD, alignment, Community-1 and automatic speaker count. Both run with network restricted and HF/Transformers/uv offline flags; no real credential is needed after caching. The app run uses its existing auto-language/Pyannote VAD defaults, so it is reported separately.

Scoring uses the installed Transformers Whisper EnglishTextNormalizer with an empty spelling-variant mapping. Case, punctuation, contractions and numbers are normalized; bracketed hesitations are removed. This is a local regression comparison, **not** the full official AppTek scoring protocol or a broad benchmark claim.

| Run | Normalized reference words | Edit count | Word error rate | Speaker-aware cpWER |
|---|---:|---:|---:|---:|
| Tiny, controlled | 156 | 19 | 12.18% | 12.18% |
| Medium, controlled | 156 | 16 | 10.26% | 10.26% |
| Medium, normal app | 156 | 17 | 10.90% | 10.90% |

cpWER minimizes per-speaker concatenated word edit distance over both possible speaker-label mappings. For predicted word midpoints falling in unambiguous reference speaker intervals, wrong-speaker counts were 0/140, 0/135 and 0/137 respectively. This does not score omitted words or establish perfect diarization; it is not time-based DER. The excerpt does not establish performance on heavy overlap, many speakers, noisy rooms, technical/code vocabulary or long meetings. Text errors remain and all technical/regulatory statements remain UNVERIFIED.

Medium improved this sample by three normalized word edits, about 1.9 percentage points WER. Keep medium available for subsequent quality tests; do not claim a universal accuracy level or make it a mandatory product default on this evidence. Existing app defaults were not changed.

Private reproducibility evidence: baseline-runtime/medium-model-provisioning.json; fixtures/apptek-ca/reference.json and DATASET_CARD.md; test-public-dialogue.ps1; evaluate-public-dialogue.py; public-dialogue-comparison.json; logs/public-dialogue-{tiny,medium}.log. The current Go model adapter and application code were unchanged during this qualification.
