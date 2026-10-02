import type { GenerationOutcome, SavedSummary, SummaryAttempt } from "@/features/transcription/hooks/useTranscriptionSummary";

// Shown while the server's own notice is not available (for example while
// free-form text is still streaming). The server sends the same wording.
export const FALLBACK_DRAFT_NOTICE =
    "UNVERIFIED AI DRAFT - not reviewed. Items are candidates, not accepted decisions or tasks. " +
    "Technical, code and regulatory statements are UNVERIFIED: a matching quote shows only that something was said, " +
    "not that it is correct. Check against the recording.";

export const KIND_ORDER = ["decision", "commitment", "suggestion", "question", "technical_claim"];

export const KIND_LABELS: Record<string, string> = {
    decision: "Decision candidates",
    commitment: "Commitment candidates (not tasks)",
    suggestion: "Suggestions and proposals (not agreed)",
    question: "Questions",
    technical_claim: "Technical, code and regulatory claims",
};

export function formatSeconds(sec: number): string {
    if (!Number.isFinite(sec) || sec < 0) return "time unavailable";
    const total = Math.floor(sec);
    const h = Math.floor(total / 3600);
    const m = Math.floor((total % 3600) / 60);
    const s = total % 60;
    const ss = String(s).padStart(2, "0");
    return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

export function formatWhen(iso?: string | null): string {
    if (!iso) return "unknown time";
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? "unknown time" : d.toLocaleString();
}

export function speakerLabel(raw: string, names: Record<string, string>): string {
    return raw
        .split(", ")
        .map(label => (names[label] ? `${names[label]} (${label})` : label))
        .join(", ");
}

// Headline for the generation the user just ran.
export function outcomeHeadline(outcome: GenerationOutcome): string {
    switch (outcome.attempt?.status) {
        case "completed":
            return outcome.attempt.mode === "freeform_v1"
                ? "Free-form draft saved - no evidence checks, UNVERIFIED"
                : "Draft saved - UNVERIFIED, needs review";
        case "rejected":
            return "Not generated - nothing was sent to the model";
        case "cancelled":
            return "Generation cancelled - nothing saved";
        case "incomplete":
            return "Generation incomplete - nothing saved";
        case "failed":
            return "Generation failed - nothing saved";
        case "running":
            return "Generation still running";
        default:
            return outcome.error ? "Generation failed - nothing saved" : "Generation finished";
    }
}

// Label for a saved summary. "Completed" never means reviewed or accurate.
export function savedStatusLabel(s: SavedSummary | null | undefined): string {
    if (!s || s.generation_status === "none") return "No saved draft";
    if (s.generation_status === "legacy_unrecorded") return "Older draft - completion was not recorded";
    if (s.format === "grounded_v1") return "Saved evidence-linked draft - UNVERIFIED, needs review";
    return "Saved free-form draft - no evidence checks, UNVERIFIED";
}

function attemptTime(a: SummaryAttempt): number {
    return new Date(a.created_at || a.started_at).getTime();
}

// A newer attempt that did not save anything, shown above the saved draft.
export function unsavedNewerAttempt(s: SavedSummary | null | undefined): SummaryAttempt | null {
    const a = s?.latest_attempt;
    if (!a || a.status === "completed") return null;
    if (!s?.created_at || s.generation_status === "none") return a;
    return attemptTime(a) >= new Date(s.created_at).getTime() ? a : null;
}

export function attemptStatusText(a: SummaryAttempt): string {
    const reason = a.reason ? ` (${a.reason.replace(/_/g, " ")})` : "";
    return `${a.status}${reason}`;
}

// Copy/download always carries the application's notice.
export function exportText(s: SavedSummary): string {
    if (s.format === "grounded_v1") return s.content; // rendered by the server with its notice
    return `> **${s.draft_notice}**\n\n${s.content}`;
}
