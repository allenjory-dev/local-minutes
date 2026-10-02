import { AlertTriangle } from "lucide-react";
import type { DraftCandidate, GroundedDraft } from "@/features/transcription/hooks/useTranscriptionSummary";
import { KIND_LABELS, KIND_ORDER, formatSeconds, speakerLabel } from "./summaryStatus";

interface GroundedDraftViewProps {
    draft: GroundedDraft;
    speakerNames: Record<string, string>;
}

// Renders a server-checked draft. Evidence (segment, speaker, time, text) comes
// from the stored transcript; only the description, quote and overview were
// written by the model. Nothing here can accept an item or verify a claim.
export function GroundedDraftView({ draft, speakerNames }: GroundedDraftViewProps) {
    const usable = draft.candidates.filter(c => !c.rejected);
    const rejected = draft.candidates.filter(c => c.rejected);
    const r = draft.report;

    return (
        <div className="space-y-6 text-sm text-[var(--text-primary)]">
            <section>
                <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-1">
                    AI-written overview - not evidence-linked
                </h3>
                <p className="leading-6 text-[var(--text-secondary)]">{draft.overview || "No overview provided."}</p>
            </section>

            <p className="text-xs text-[var(--text-tertiary)]">
                {r.items} candidates: {r.usable} cite existing transcript segments ({r.quotes_matched} with the quote found in a cited segment),
                {" "}{r.flagged} flagged for review, {r.rejected} rejected by validation. Every item needs human review; none is an accepted
                decision or task.
            </p>

            {usable.length === 0 && (
                <p className="italic text-[var(--text-tertiary)]">No candidates were extracted.</p>
            )}

            {KIND_ORDER.map(kind => {
                const items = usable.filter(c => c.kind === kind);
                if (items.length === 0) return null;
                return (
                    <section key={kind}>
                        <h3 className="font-semibold text-base mb-2">
                            {KIND_LABELS[kind] ?? kind}
                            {kind === "technical_claim" && <span className="ml-2 text-xs font-bold text-red-600 dark:text-red-400">UNVERIFIED</span>}
                        </h3>
                        <ul className="space-y-3">
                            {items.map(c => <CandidateCard key={c.id} candidate={c} speakerNames={speakerNames} />)}
                        </ul>
                    </section>
                );
            })}

            {rejected.length > 0 && (
                <details className="rounded-xl border border-[var(--border-subtle)] p-3">
                    <summary className="cursor-pointer text-[var(--text-secondary)]">
                        Rejected by validation ({rejected.length}) - no usable transcript evidence
                    </summary>
                    <ul className="space-y-3 mt-3 opacity-80">
                        {rejected.map(c => <CandidateCard key={c.id} candidate={c} speakerNames={speakerNames} />)}
                    </ul>
                </details>
            )}
        </div>
    );
}

function CandidateCard({ candidate: c, speakerNames }: { candidate: DraftCandidate; speakerNames: Record<string, string> }) {
    const showOwner = c.kind === "commitment" || c.kind === "decision";
    return (
        <li className="rounded-xl border border-[var(--border-subtle)] bg-[var(--bg-main)] p-3 space-y-2">
            <div className="flex flex-wrap items-start gap-2">
                <span className="font-medium flex-1 min-w-[12rem]">{c.text || "(no description)"}</span>
                {c.rejected && <span className="text-xs rounded-full px-2 py-0.5 bg-[var(--bg-card)] text-[var(--text-tertiary)]">{c.kind}</span>}
                <span className="text-xs rounded-full px-2 py-0.5 bg-amber-500/15 text-amber-700 dark:text-amber-400">Needs review</span>
                {c.verification_status && (
                    <span className="text-xs font-bold rounded-full px-2 py-0.5 bg-red-500/15 text-red-700 dark:text-red-400">{c.verification_status}</span>
                )}
            </div>
            {showOwner && (
                <p className="text-xs text-[var(--text-secondary)]">
                    Owner: {c.owner === "Not stated" ? c.owner : speakerLabel(c.owner, speakerNames)} · Due: {c.due}
                </p>
            )}
            {c.quote && (
                <p className="text-xs text-[var(--text-secondary)]">
                    <span className="italic">“{c.quote}”</span>{" "}
                    <span className={c.quote_matched ? "text-[var(--text-tertiary)]" : "font-semibold text-amber-700 dark:text-amber-400"}>
                        {c.quote_matched ? "- quote found in the cited segment" : "- quote NOT found in the cited segment"}
                    </span>
                </p>
            )}
            {c.evidence.length > 0 && (
                <ul className="space-y-1">
                    {c.evidence.map(e => (
                        <li key={e.segment_id} className={`text-xs rounded-lg px-2 py-1 ${e.contains_quote ? "bg-[var(--bg-card)]" : ""}`}>
                            <span className="font-mono text-[var(--text-tertiary)]">{e.segment_id}</span>
                            {" · "}{speakerLabel(e.speaker, speakerNames)}
                            {" · "}{formatSeconds(e.start)}-{formatSeconds(e.end)}
                            <span className="block text-[var(--text-secondary)]">{e.text}</span>
                        </li>
                    ))}
                </ul>
            )}
            {c.flags.length > 0 && (
                <ul className="space-y-1">
                    {c.flags.map((f, i) => (
                        <li key={`${f.code}-${i}`} className="flex gap-1.5 text-xs text-amber-800 dark:text-amber-300">
                            <AlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" aria-hidden="true" />
                            <span>{f.message}</span>
                        </li>
                    ))}
                </ul>
            )}
        </li>
    );
}
