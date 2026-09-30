# Roadmap

1. **Current: establish baseline.** Inspect existing system; assess source/license/alternatives; fork with develop; isolated installation; short audio, CUDA, diarization, playback, persistence and offline tests. Record failures honestly and stop for approval.
2. **First recommended change, subject to approval:** preserve uploaded originals byte-for-byte and write conversions as derivatives, with hashes and a focused WebM regression test. This closes a concrete conflict with the product's core promise.
3. Fix credential-bearing command logging before real Hugging Face credentials; pin engine/model/dependency revisions and make environment readiness truthful. Establish repeatable Windows installation and local-only inference.
4. Versioned session package import/export, provenance and configurable profiles/markers. Keep SQLite as a convenience index.
5. UNVERIFIED technical-claim records and explicit authoritative-source verification; carry state through summaries and exports.
6. Improve local summaries and library search only after baseline reliability; measure speaker accuracy on realistic recordings.
7. Android capture and transfer design after desktop package contract stabilizes. RODE hardware testing and personal workflow configuration are later work.

Steps 2 onward require approval. No UI rebranding or broad refactor is necessary to prove the foundation.
