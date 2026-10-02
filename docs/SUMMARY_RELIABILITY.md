# Summary reliability increment 1 - 2026-10-02

Branch `feature/summary-reliability-v1` from `develop` at `fe80669`. Claude implemented it; ChatGPT/Codex review and runtime verification are pending. **This increment does not make summaries accurate.** It makes generation status truthful, keeps transcript text out of logs, sets the UNVERIFIED and draft status in application code, and checks model references against the stored transcript. Semantic accuracy still needs human review and a fresh qualification on the real model.

No live model, GPU, recording, private transcript or runtime database was used. All tests use synthetic transcripts and mocked provider responses.

## Decision

**Context.** Local qualification (LOCAL_SUMMARIES.md) failed:

- suggestions became actions;
- quotes were given to the wrong speaker, and paraphrases appeared in quotation marks;
- disagreement was invented or material omitted;
- UNVERIFIED labels were inconsistent.

Prompt changes did not fix this. Source review also found transcript text in logs, no completion check on streams, and partial output saved as the current summary.

**Decision.** Keep the model as a draft generator and move every status and reference decision into server code:

1. **Evidence-linked drafts.** The server builds the input from the stored transcript. Each segment line is `[S#] SPEAKER_xx: text`, inside a delimited data block that is separate from the app-owned instructions. No timestamps are sent. The model must return JSON candidates: kind, text, segment IDs, quote, speaker, owner and due date. The server then:
   - checks every reference against the stored transcript;
   - takes the speaker, timestamps and evidence text from the transcript, not from the model;
   - rejects or flags anything that does not match.
2. **Attempts are separate from summaries.** Every generation is recorded with an explicit outcome. Only a completed, parseable result creates a summary row. Failures never replace the previous one.
3. **Draft status comes from the application.** The draft notice, review state (`needs_review`) and verification state (`UNVERIFIED`) are constants in code. There is no code path or API that marks an item accepted or a claim verified.
4. **Oversized input is rejected.** It is never truncated.

**Alternatives not taken.**

- More prompt work: already shown not to work.
- A larger or reasoning model: needs new downloads, which were out of scope, and the reasoning model ran out of output budget.
- Long-meeting chunking: out of scope for this increment.
- Automatic semantic judgement: impossible to verify honestly.

**Consequences.**

- Model mistakes become visible flags instead of silent text.
- The checks are lexical. They catch common phrasings of the observed failures and miss others.
- The evidence-linked mode uses built-in instructions; the template supplies only the model.
- Free-form templates still work, with honest status, but are labelled as having no evidence checks.

## What changed

| Area | Change |
|---|---|
| Privacy | Removed the Ollama stream's debug print of up to 2,000 request characters, which included transcript text (this affected chat too). OpenAI error bodies are no longer logged. GORM now logs SQL without bound values: its default logger printed full values for failed or slow (over 200 ms) writes, app-wide. Summary logs carry only IDs, mode, model, status, reason, sizes, token counts and durations. |
| Provider stream | New `StreamWithOutcome` for Ollama and OpenAI-compatible providers. It uses an ordered callback (no channel close-order race) and reports explicit completion, `done_reason`/`finish_reason`, token counts, in-stream errors, HTTP errors, unreadable lines and cancellation. Provider messages are kept for display but left out of `Error()`. |
| Status | Possible outcomes: `completed`, `failed`, `incomplete`, `cancelled`, `rejected`, and `running` while generating. A `running` attempt is reported as interrupted after a restart. Reasons are listed below. |
| Limits | Context 8,192 and output 2,048 tokens by default, matching the Modelfile. They are sent to Ollama as `num_ctx`/`num_predict` with each request. The input estimate is conservative (it rounds up). Override with `LOCAL_MINUTES_SUMMARY_CONTEXT_TOKENS` and `LOCAL_MINUTES_SUMMARY_MAX_OUTPUT_TOKENS`; invalid values fail visibly. |
| API | `POST /api/v1/transcription/{id}/summary/grounded` creates an evidence-linked draft. `GET .../summary/attempts/{attempt_id}` returns one attempt's outcome. `GET .../summary` adds `generation_status`, `format`, `draft_status`, `draft_notice`, `draft`, `transcript_changed` and `latest_attempt`. `POST /api/v1/summarize` (free-form) keeps its request and stream but saves only confirmed-complete text and returns `X-Summary-Attempt-Id`. |
| UI | Summary dialog only. Output choice: "Evidence-linked draft" (default) or "Free-form template text". The dialog shows the status of the attempt just run, the previous draft kept, a newer unsaved attempt, transcript-changed notices and the always-visible notice. Evidence-linked items show "Needs review", UNVERIFIED, quote match, cited segments with transcript speaker and times, flags, and a rejected list. Model output no longer renders raw HTML. Copy and download export only the saved draft, with the notice. |

### Reasons recorded on attempts

| Status | Reasons |
|---|---|
| `failed` | `provider_error`, `provider_unreachable`, `empty_output`, `malformed_output`, `storage_error` |
| `incomplete` | `stream_ended_without_completion`, `stream_lines_unreadable`, `output_limit_reached`, `context_limit_exceeded`, `unexpected_finish_reason`, `output_too_large`, `timed_out`, `interrupted` |
| `cancelled` | `cancelled` (client disconnected or closed the request) |
| `rejected` | `input_too_large`, `transcript_has_no_segments` (no model call) |

## Checks on evidence-linked candidates

Kinds: `decision`, `commitment`, `suggestion` (this includes proposals that were not agreed), `question`, `technical_claim`.

Items are rejected (kept in a separate list, never presented as findings) when:

- the kind is unknown;
- the item could not be read;
- no segment is cited, or no cited segment exists;
- the quote is missing.

Every other item stays `needs_review`, with flags for:

- **Quote checks:** the quote's words are not found in the cited segment(s), with the location if found elsewhere; the quote has an ellipsis gap; the quote spans several speakers.
- **Number qualifiers:** the quote starts or ends next to a word that changes its number, such as "5 kPa" taken from "minus 5 kPa", "not more than 5 kPa" or "5 kPa or more". The quote still counts as found; the flag names the left-out words.
- **Attribution:** the model's speaker differs from the transcript's speaker.
- **Owner:** the owner is a speaker label but did not speak the quoted words, or the owner is not in the cited text.
- **Deadline:** not in the evidence; contradicted ("Tuesday, not Monday"); or possibly corrected later, including when the same speaker corrects themselves.
- **Commitments:** no explicit first-person commitment or accepted assignment, suggestion wording, or negative wording.
- **Decisions:** questions, proposals, refusals, or no agreement wording.
- **Instruction-like text:** the cited segment contains text addressed to an AI, such as "ignore previous instructions".
- **Technical content:** code, standard, clause, numbers with units or pressure wording. These items are always `UNVERIFIED`, and a spoken "I verified it" is flagged, never accepted.

Missing or placeholder owners and deadlines become `Not stated`. Matching words shows only that something was said, never that it is true or agreed. Exact match ignores case and punctuation, but not word choice: "I'll" does not match "I will", and "should" does not match "can". A quote may continue across adjacent segments, never non-adjacent ones.

Numbers are the exception to "punctuation is ignored". Every symbol that can change a number stays part of it, with or without a space in between: a sign, comparison mark or currency sign before it, a leading point, any symbol between digits, and %, °, + or a currency sign after it. So "-5 kPa" and "− 5 kPa" do not match "+5 kPa" or "5 kPa", "1/2 inch" does not match "1.2 inch", ".5" does not match "5", and "Clause 7.2" does not match "Clause 7.2.3". Spacing alone does not matter: "− 5" equals "-5", "5 %" equals "5%", and "5 - 10" is the range "5-10". The Unicode minus sign and dashes count as "-", and "½" counts as "1/2". Letters still split from digits, so "300mm" matches "300 mm".

A dash before a number counts as a minus sign unless it is inside a word ("COVID-19") or between two numbers (a range). A quote that leaves out such a dash is therefore reported as not found, even where the dash was only punctuation.

Words are different: "5 kPa" really is in "minus 5 kPa", so the quote stays matched and the number-qualifier flag is added instead. The check looks at the words directly before a quote that starts at a number, and directly after a quote that ends at a number or a number and its unit. Stacked words are followed back ("not more than"). A bare number can also lose its unit and a qualifier ("5" taken from "5 kPa or more"). The same speaker's segment just before or after the cited ones is included, because ASR can split a phrase. Only the first place the quote is found is checked.

**Qualifier words (first draft, needs sign-off before merge):**

- Before a number: minus, negative, plus, plus or minus; less / more / fewer / greater / lower / higher than, and the same with "no" in front; under, over, below, above, up to, at least, at most, within, between; maximum, max, minimum, min, exceed, exceeds, exceeding; about, around, approximately, approx, roughly, nearly, almost, close to; not, never, isn't, aren't, wasn't, weren't, don't, doesn't, didn't, won't, can't, cannot, shouldn't.
- After a number: or less, or more, or fewer, or lower, or higher, or greater, or below, or above, or under, or over, or so; and up, and above, and below, and under, and over; maximum, max, minimum, min, at most, at least; below, above, under, over, less, more, lower, higher; plus, minus, ish, short.
- Number words that count as numbers: zero to twenty, thirty to ninety, hundred, thousand, million, half, quarter, dozen.

A bare "no" is left out because "No, 5 kPa" is a common answer. Words such as "about" and "over" will also flag ordinary phrases ("What about 5 kPa?", "hand over 5 copies"); strike them if that is too noisy.

## Schema, backup and rollback

GORM AutoMigrate makes additive changes only:

- `summaries` gains `generation_status` (default `legacy_unrecorded`), `format` (default `legacy_markdown`), `provider`, `attempt_id`, `transcript_sha256` and `draft_json`.
- A new `summary_attempts` table is created.

Existing rows are kept and labelled as legacy drafts whose completion was not recorded. Earlier versions could save partial output, so the label is deliberately cautious.

**Before first start of a build with this change:**

1. Stop only the identified Local Minutes server, with no active jobs.
2. Take a consistent copy of the SQLite database, including its `-wal` and `-shm` files while stopped.
3. Keep the previous executable.

**Rollback:**

1. Stop the new server.
2. Start the previous executable against the same database. It ignores the new columns and table, and rows it writes are reported as legacy if the new build runs again.
3. Restore the backup only to undo data written after the upgrade. Restoring discards later recordings.

Independent review ran the base-commit binary on a migrated database and confirmed it starts and writes. Runtime verification on the Windows installation is still required.

**Pre-existing, not changed:** independent review observed that GORM rebuilds `transcription_jobs` on every start, with SQLite. This predates the branch; Codex should confirm and assess it separately.

## Verification performed (Linux sandbox, synthetic only)

| Check | Result |
|---|---|
| Go baseline before changes | PASS: 245 passed, 0 failed. The two Windows-documented failures do not reproduce on Linux. |
| Full Go suite after changes | PASS: 303 passed, 0 failed, 0 skipped (`go test ./... -count=1`) |
| Race detector on new packages and summary API tests | PASS |
| Mutation checks | PASS: disabling the language, deadline, speaker-derivation or technical-status checks, persisting failed output, removing the input bound, logging model output, or skipping stale-attempt reconciliation each made the new tests fail |
| Frontend | PASS: `tsc -b`, `eslint src/features/transcription` and `npm run build` |
| Rendered UI (real router and built frontend, scripted Ollama stand-in, headless Chromium) | PASS for legacy draft notice, evidence-linked draft with flags/UNVERIFIED/speaker names/source times, an output-limit failure keeping the previous draft, free-form save and failure with partial text marked "not saved", model HTML shown as text, reopen, and phone width. The harness was not committed. |
| Old handler, same synthetic burst (400 chunks then done) | Output lost in 38 of 40 runs, in both response and saved summary: the close-order race is real for fast streams. Real-world frequency on the GPU stream was not measured. New handler: 25 of 25 repeats complete. |
| Independent code review (separate agent) | 11 findings. All fixed in `c99a790` with tests, except the documented limitations below. |
| Real model / GPU / Windows | Not run here. Reported by the independent review on the laptop: Windows build passed, all eight synthetic meetings produced local drafts, and the migration preserved existing data on a backup copy. It found the two problems fixed in the next row. |
| Correction round: numeric quotes and an open test database | PASS: 304 passed, 0 failed. The new number cases fail with the previous tokenizer. On Linux, an open-file check now fails the database tests if a file is still open at the end. Built as Windows test binaries and run under Wine (not a real Windows install): the previous database test fails with the same "sharing violation" cleanup error; the fixed one passes, as do the summary, LLM and summary API suites. |
| Second review on the laptop | Both fixes pass on Windows, and two local-model drafts kept all five numeric quotes. A full Windows run had one intermittent failure in a timeout test that passed on isolated reruns. This branch's tests have no timing thresholds besides 10-second safety waits. The only test named for a timeout is the upstream `TestGetModelsTimeout` (a 1 ns deadline against a local mock server), which this branch does not change; it passed 3,000 Linux runs and 300 Wine runs. The review found "− 5 kPa" (minus sign, space, digit) still matched "5 kPa". |
| Follow-up: spaced signs and symbols | PASS: 305 passed, 0 failed. The new cases fail with the previous tokenizer; all fixture outcomes are unchanged. The summary tests and summary API suites also pass as Windows binaries under Wine. |
| Third review on the laptop | The spaced-symbol fix and the database cleanup tests pass on Windows. The full suite fails only `TestLLMTestSuite/TestGetModelsTimeout`, one of the two baseline failures recorded in `BASE_PROJECT_ASSESSMENT.md` before this work. The review asked for a separate warning when a quote leaves out a word such as "minus", keeping the literal match. |
| Follow-up: number-qualifier warning | PASS: 306 passed, 0 failed. The new test fails when the check is disabled, when stacked words are not followed, when the trailing side is skipped, and when the neighbouring segment is ignored or used for any speaker. All fixture outcomes are unchanged. |

## Limitations

- **Semantic accuracy is not solved.** The lexical checks miss differently phrased suggestions, commitments, refusals and corrections. Omissions cannot be detected. The overview is model-written and unchecked.
- **No live-model verification.** Whether `local-minutes-summary:7b` returns valid schema-constrained JSON and useful candidates is untested. Prompt version `lm-grounded-2026-10-02` has not been qualified.
- **Number formats must match exactly.** "1,200" and "1200", or "3.30" and "3:30", are reported as not found, and spelled-out numbers ("five") never match digits. These items need review rather than being matched wrongly.
- **The number-qualifier flag is a word list.** It catches only the listed words, directly next to the quote, at the first place the quote is found. Other phrasing ("don't go over 5 kPa" quoted as "over 5 kPa") is missed. A symbol split from its number by a segment boundary ("set it to -" then "5 kPa") is not caught.
- **No long-meeting support.** Long meetings are rejected. The estimate is deliberately pessimistic (often 30-60% above actual for English), so some transcripts that would fit are rejected.
- **OpenAI-compatible providers:** the context size and JSON schema are not sent, and usage is usually unreported. A server with a smaller context than 8,192 could still truncate input; it is only caught if the server reports an error.
- **Reviewing is not implemented.** There are no accept/dismiss controls, so every item stays "needs review". Authoritative-source verification is not implemented either: technical claims stay UNVERIFIED with no way to change that, by design for now.
- **Free-form mode** still sends client-built content and has no evidence checks or transcript fingerprint.
- **Chat** still uses the older channel stream; only its content logging was removed.
- **Not regenerated or added:** Swagger `api-docs/` was not regenerated, and timestamps do not seek audio.
- **Old logs are unchanged.** Runtime logs written before this build may still contain transcript text from the removed debug print. Treat them as private and do not upload them.

## Independent review and runtime test plan (ChatGPT/Codex)

1. **Fetch the branch without touching the working tree.**
   - `git fetch <bundle-path> feature/summary-reliability-v1:feature/summary-reliability-v1`, unless the branch was already added.
   - Review with `git log --oneline fe80669..feature/summary-reliability-v1` and `git diff fe80669..feature/summary-reliability-v1`.
2. **Review the code.** Focus on:
   - `internal/llm/outcome.go` and both providers' `StreamWithOutcome`;
   - `internal/summary/` (validation rules in `validate.go` and `text.go`, classification in `outcome.go`);
   - `internal/api/summary_attempts.go` and `summarize_handlers.go`;
   - `models/summary.go`, `repository/implementations.go` and `database/database.go`;
   - `web/frontend/src/features/transcription/`.

   Look for any path that saves or labels a failed result as completed, any content in logs, and any way to reach a verified or accepted state.
3. **Run the automated checks on Windows.**
   - `go test ./... -count=1`. Compare against the two known baseline failures. The open-database cleanup failure in `internal/database` should be gone.
   - `go test -race` on `./internal/llm ./internal/summary ./internal/database ./tests` if a C toolchain exists.
   - In `web/frontend`: `npm ci`, `npx tsc -b`, `npx eslint src/features/transcription`, `npm run build`.
4. **Build without replacing the running app.** Copy `web/frontend/dist` to `internal/web/dist`, then build a new executable name.
5. **Rehearse on a database copy first.**
   - Confirm the new columns and table, and that existing summaries show "Older draft - completion was not recorded".
   - Confirm transcripts, audio and the job count are unchanged.
6. **Run the real model.** Use `local-minutes-summary:7b` on the earlier synthetic transcripts (commitment vs suggestion, no-action small talk, corrected deadline plus injection) and the public samples. For each, record:
   - JSON validity and time;
   - candidates versus the source;
   - flags raised and false negatives.

   Re-score ACCEPTANCE_TESTS.md honestly. Expect PARTIAL or FAIL on semantics.

   Also confirm two assumptions:
   - Ollama uses the request's system message instead of the Modelfile `SYSTEM` prompt, so the eight-section Markdown instructions are not mixed in.
   - `format` (the JSON schema) is honoured by Ollama 0.35.0.
7. **Exercise the failure paths.** Each must report its stated outcome and keep the previous draft:
   - stop Ollama, which should give "provider could not be reached";
   - set `LOCAL_MINUTES_SUMMARY_MAX_OUTPUT_TOKENS=256` on a longer transcript, which should give the output limit;
   - use a long transcript, which should give a 413-style "too long", with nothing sent;
   - close the browser mid-generation, which should give cancelled;
   - restart while generating, which should give interrupted.
8. **Check privacy and data.**
   - After the runs, search the new runtime logs for distinctive transcript and model phrases. Only `[summary]` metadata lines are expected.
   - Confirm original audio hashes, the OpenWhispr inventory and transcription of a short public fixture are unchanged.
9. **Check persistence.** Restart the server and reopen the recording: the saved draft, flags and attempt history must persist.
