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

2026-09-30: add Local Minutes product requirements, developer rules, acceptance criteria, environment/architecture assessment, foundation decision and roadmap. No application feature changes, attribution removal or transcription-engine replacement.

2026-09-30: add build-versus-reuse judgment, setup/handoff notes and observed native setup risks. Baseline binary built from the original upstream application revision. Local runtime uses WhisperX commit `771b4a14a9486f8fd5aef18ef49e35d639523dd3` as fetched by upstream initialization; its own license is BSD-2-Clause and remains in its runtime checkout.

Runtime toolchains, models, user data, machine inventory and secrets are excluded from public Git history. Baseline runtime dependencies and results are recorded separately. Model licenses and access conditions must be tracked independently from the application's MIT license.

2026-09-30: record the user's Obsidian/personal-assistant requirement, proposed export contract and acceptance gates. No export feature or LifeBot code changes. A dated project-document snapshot is saved separately to the user's configured local vault.
