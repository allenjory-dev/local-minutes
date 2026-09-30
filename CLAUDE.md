# Local Minutes development rules

- Read PRODUCT_SPEC.md, docs/UPSTREAM.md, docs/FOUNDATION_DECISION.md and current baseline results before coding.
- Do not invent features or silently change architecture. Make small, scoped, reviewable changes.
- Preserve working functionality. Do not replace transcription engines without user approval.
- Never modify original audio. Processing output is a derivative with provenance.
- Preserve local-first operation. No mandatory subscription or external AI API.
- Spoken technical/code/regulatory claims remain UNVERIFIED until a user explicitly verifies them against an authoritative source.
- Do not hard-code personal workflows into the generic product. Marker types and profiles are configurable.
- Obsidian export is a required future adapter; configure vault paths and assistant access separately. Preserve user edits, source timestamps and UNVERIFIED labels. Do not assume local vault storage makes a cloud-backed assistant local or always available.
- Never install into or repair existing global Python, CUDA, Whisper, ffmpeg or model environments as a side effect. Inspect and document before dependency changes; use isolated project environments.
- Never publish recordings, machine inventory, tokens, secrets, model caches or runtime logs.
- Preserve upstream LICENSE, copyright, attribution and dependency/model notices.
- Develop on develop or a scoped branch from it. origin is the user's repository; upstream is read-only. Do not push upstream.
- Run meaningful tests before declaring work complete. Distinguish source-supported, tested, blocked and failed behavior. A green build is not proof of GPU inference or speaker quality.
- Document meaningful architecture decisions and upstream changes. Prefer small commits with clear messages.
- Claude: primary implementation. ChatGPT: architecture review, QA, verification and testing.
- Baseline scope ends after installation/testing/reporting. Do not start UI redesign, Android, Done By the Book integration or feature development without approval.
