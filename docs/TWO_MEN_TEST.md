# Two male speakers: baseline qualification

Date: 2026-10-02. User requested a male-male conversation after the earlier male-female public test. No application code, model, dependency, global environment or defaults changed.

## Source and selection

[AppTek Call-Center Dialogues](https://huggingface.co/datasets/apptek-com/apptek_callcenter_dialogues), CC-BY-SA-4.0, human role-played conversation with supplied transcript, speaker intervals and gender metadata. Dataset revision `b98967d9946f7f59f58d08624a2a00fe98fe0219`. Canadian, general American and British subsets had no conversations with two distinct metadata-labelled male speakers. Australian English had nine; selected the lexicographically first before inference: `diarization/en-AU/audio/en_AU_Aviation_1585418.wav`.

Downloaded one 809.25-second original, preserved it unchanged, and created a separate 86.79-second mono 16 kHz WAV excerpt ending at a clean reference boundary nearest 90 seconds. Only the excerpt was processed. Source SHA256: `a2096362bd905deb7430b2c6ff5ce9c5dce01a3df9d7b8fc37ac9dabd086810c`. Excerpt SHA256: `539e94d5f1b07425f19f17a1a5346ec68f1d24caf5452260544f3d95c4f25c75`. License/card retained locally; audio and full transcripts are not redistributed in Git or Obsidian.

Reference has 11 speech segments, **three speaker turns / two speaker changes**, no overlapping reference speech, and 180 normalized reference words. Male classification comes from dataset metadata, not an inference about identity from voice. This is a same-gender check; acoustic similarity was not measured.

## Configuration and results

Existing WhisperX medium, CUDA float16, batch 2, word alignment, pyannote Community-1, **automatic speaker count** (no count supplied). Controlled run uses English/Silero VAD and cached models under network restrictions plus HF/Transformers/uv offline flags. Normal app run uses existing auto-language/Pyannote VAD defaults and its external credential loader; token field explicitly empty.

| Test | Result |
|---|---|
| Controlled offline pipeline | PASS: 16.69s, 17 segments, 176 timed words, two speakers |
| Normal app pipeline | PASS: 15.51s, 18 segments, 173 timed words, two speakers |
| Speaker assignment | PASS on scored words: 0/176 wrong in controlled run; 0/173 wrong in app run |
| Normalized transcription WER | Controlled 19/180 edits = 10.56%; app 14/180 = 7.78% |
| Speaker-aware cpWER | Same as WER in each run |
| Timestamp bounds | PASS: every predicted word labelled and within excerpt bounds |
| Listen/playback | PASS: selecting “Thanks, Jim.” sought to 21.542s; playback advanced to 50.430s and paused, no media error |
| Save/reopen | PASS: completed transcript persisted and browser reload reopened it |
| Original preservation | PASS: downloaded original and uploaded excerpt hashes match; original_audio_path retained |
| Busy meeting / similar voices / interruptions | PARTIAL: not established by this sample |

App job: `823b4033-5410-4965-b7ba-5c30a62d024c`, title `public-two-men-dialogue`. Saved job credential field verified empty. Prior sessions retained.

## Interpretation and limits

Speaker labels were optimally mapped to the supplied reference, then each predicted word midpoint was compared with the reference speaker at that time. All predicted words fell within unambiguous reference intervals; none were assigned to the wrong speaker. This excludes omitted words and does not establish perfect diarization or measure time-based DER.

WER uses the same local Transformers Whisper EnglishTextNormalizer as the earlier test, not the full official AppTek protocol. cpWER minimizes concatenated per-speaker word edits across label permutations. Text errors remain. This different recording cannot isolate gender as a causal difficulty factor or establish that male-male speech is universally easier/harder. It supports keeping the existing foundation; no engine replacement is justified by this result.

Next useful validation: a short consented same-room recording with similar-sounding speakers, several exchanges and brief interruptions. The user's phone/RODE hardware, long meetings and noisy rooms remain untested. All extracted technical/regulatory claims remain **UNVERIFIED**.

Private evidence under baseline-runtime: fixtures/apptek-two-men/reference.json and DATASET_CARD.md; prepare-two-men-dialogue.py; test-two-men-dialogue.ps1; evaluate-two-men-dialogue.py; two-men-dialogue-comparison.json; two-men-dialogue-app-result.json; logs/two-men-medium.log. Harness: local-minutes/.local-minutes/two-men-dialogue-smoke.go. No actual credential is used for the cached offline run.
