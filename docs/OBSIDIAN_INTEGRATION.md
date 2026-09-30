# Obsidian export and assistant retrieval

Requested 2026-09-30. Status: approved product requirement; design proposal only. No meeting export automation or LifeBot runtime change has been implemented. Baseline qualification remains the immediate task.

## User outcome

After a meeting or conference, the user should be able to ask their personal assistant what happened, what was decided, what needs following up, or where something was said. The assistant should retrieve the saved record and cite the session/date and timestamp, distinguish a speaker's statement from a verified fact, and say when evidence is missing.

## Proposed data flow

Local Minutes portable session package -> optional Markdown export adapter -> configured Obsidian vault -> existing assistant search/read tools -> answer with source references.

No new transcription engine or mandatory hosted database is needed for this integration. The portable package retains original audio and machine-readable results. Obsidian provides a durable, readable projection. Local Minutes remains useful without Obsidian or LifeBot installed.

## Proposed vault representation

Configurable example:

```text
Meetings/
  YYYY-MM-DD - Session title - short-id/
    Session.md
    Transcript 001.md
    Transcript 002.md
    Markers.md
```

Session.md contains metadata and links, executive summary, major takeaways, decisions, actions, follow-ups, questions and ideas. Transcript parts contain speaker labels, text and original-audio timestamps; each links to the index and adjacent parts. Markers retain IDs/types/times and optional user text. Human-renamed speakers do not establish verified identity.

Frontmatter should include a schema version, stable session ID, session date/time/timezone, type, source audio hash, processing revision, export time, and verification/access metadata. An AI-generated summary is labelled as such and links to evidence. Speaker statements about codes/standards/dimensions remain UNVERIFIED unless explicit authoritative-source verification is carried with them.

## Current assistant compatibility

Read-only inspection of the user's LifeBot on 2026-09-30 confirmed a configured local Obsidian vault and existing `search_vault_notes`, `read_vault_note` and project-reader functions. No extra ingestion service is necessary to make a new Markdown note available to those local tools. The project reader caps at 5,000 characters; general note reads cap at 30,000. Design transcript parts below that limit with headroom (for example, 20,000 characters), explicit continuation links and complete coverage checks. Search returns snippets, so answering from a search hit alone is insufficient.

Local file retrieval is not proof that natural-language routing, all transports or remote question answering work. Qualify those separately with non-sensitive fixtures before real meeting content. Do not send private recordings or notes to an external model merely to test the integration.

## Durability, updates and conflicts

- Stable session IDs identify exports even if titles change.
- Re-export updates generated sections/files only; user notes are retained separately or with protected boundaries.
- Detect concurrent edits and surface conflicts rather than silently overwriting.
- Write atomically where supported; read back/hash-check before declaring success. Track failed/pending exports with safe retries.
- Keep verification records distinct from generated claims; changed claims invalidate prior verification.
- A saved action item is evidence from the meeting, not authorization to create a calendar event or reminder.
- Vault backup/sync is a separate responsibility. Do not promise permanent availability without a verified backup and recovery plan.

## Privacy and remote availability

The inspected LifeBot configuration enables cloud-backed interactive model routing. Consequently, local Obsidian storage does not by itself keep a retrieved answer's context on the laptop. Before automatic export of sensitive meetings, choose and verify either enforceable assistant exclusion/local inference or explicitly approved cloud processing. A frontmatter flag without enforcement is insufficient because existing vault search scans Markdown broadly.

Remote text answers require the assistant host/transport to be reachable. A sleeping laptop can prevent access. Phone playback additionally needs a secure reachable audio route or approved audio sync; do not expose a public server or change firewall/network policy as an implicit part of export. A local Windows file link is useful on the laptop but not a universal phone link.

## First integration acceptance scenario (future)

Export a synthetic two-speaker meeting, ask what decision was made, and retrieve the exact session/date/timestamp. Then test a later transcript part, two similarly titled sessions, an UNVERIFIED code claim, corrected/re-exported content, a failed export retry and a private-session exclusion. Verify desktop and remote behavior separately. Keep the existing baseline account and diarization blockers visible until resolved.
