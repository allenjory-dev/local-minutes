import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/features/auth/hooks/useAuth";
import { useCallback, useState } from "react";

export interface SummaryTemplate {
    id: string;
    name: string;
    model: string;
    prompt: string;
    include_speaker_info?: boolean;
}

// Statuses are assigned by the server. "completed" only means generation
// finished and was saved; it never means the content is accurate or reviewed.
export type AttemptStatus = "running" | "completed" | "failed" | "incomplete" | "cancelled" | "rejected";
export type GenerationStatus = AttemptStatus | "legacy_unrecorded" | "none";

export interface SummaryAttempt {
    id: string;
    transcription_id: string;
    mode: string;
    provider: string;
    model: string;
    status: AttemptStatus;
    reason?: string;
    detail?: string;
    finish_reason?: string;
    input_tokens_estimate: number;
    input_token_budget: number;
    output_chars: number;
    summary_id?: string;
    started_at: string;
    finished_at?: string;
    created_at: string;
}

export interface DraftEvidence {
    segment_id: string;
    speaker: string;
    start: number;
    end: number;
    text: string;
    contains_quote: boolean;
}

export interface DraftFlag {
    code: string;
    message: string;
    segment_id?: string;
}

export interface DraftCandidate {
    id: string;
    kind: string;
    text: string;
    quote: string;
    quote_matched: boolean;
    evidence: DraftEvidence[];
    speaker: string;
    model_speaker?: string;
    owner: string;
    due: string;
    review_state: string;
    verification_status?: string;
    flags: DraftFlag[];
    rejected: boolean;
}

export interface GroundedDraft {
    schema_version: string;
    prompt_version: string;
    overview: string;
    candidates: DraftCandidate[];
    report: { items: number; usable: number; rejected: number; flagged: number; quotes_matched: number };
    transcript_sha256: string;
    segment_count: number;
}

export interface SavedSummary {
    id?: string;
    transcription_id: string;
    template_id?: string | null;
    model: string;
    provider?: string;
    content: string;
    created_at?: string | null;
    updated_at?: string | null;
    generation_status: GenerationStatus;
    format?: string;
    draft_status: string;
    draft_notice: string;
    draft?: GroundedDraft | null;
    transcript_changed?: boolean;
    latest_attempt?: SummaryAttempt | null;
}

// The outcome of the generation the user just started.
export interface GenerationOutcome {
    attempt: SummaryAttempt | null;
    error: string | null;
    previousSummaryKept?: boolean;
}

export function useSummaryTemplates() {
    const { getAuthHeaders } = useAuth();
    return useQuery({
        queryKey: ["summaryTemplates"],
        queryFn: async () => {
            const response = await fetch("/api/v1/summaries", {
                headers: getAuthHeaders(),
            });
            if (!response.ok) throw new Error("Failed to load summary templates");
            return response.json() as Promise<SummaryTemplate[]>;
        },
        staleTime: 5 * 60 * 1000, // Templates don't change often
    });
}

export function useExistingSummary(audioId: string) {
    const { getAuthHeaders } = useAuth();
    return useQuery({
        queryKey: ["summary", audioId],
        queryFn: async () => {
            const response = await fetch(`/api/v1/transcription/${audioId}/summary`, {
                headers: getAuthHeaders(),
            });
            if (!response.ok) throw new Error("Could not load the saved summary.");
            return response.json() as Promise<SavedSummary>;
        },
        retry: false,
    });
}

async function readJSON(res: Response): Promise<Record<string, unknown> | null> {
    try {
        return (await res.json()) as Record<string, unknown>;
    } catch {
        return null;
    }
}

function outcomeFromBody(body: Record<string, unknown> | null, fallback: string): GenerationOutcome {
    return {
        attempt: (body?.attempt as SummaryAttempt | undefined) ?? null,
        error: (body?.error as string | undefined) ?? fallback,
        previousSummaryKept: body?.previous_summary_kept as boolean | undefined,
    };
}

// Free-form template generation. The text streams as it is generated, but it
// is only saved if the server confirms completion; the attempt is read back
// after the stream ends to learn what actually happened.
export function useSummarizer(audioId: string) {
    const { getAuthHeaders } = useAuth();
    const queryClient = useQueryClient();
    const [isStreaming, setIsStreaming] = useState(false);
    const [streamContent, setStreamContent] = useState("");
    const [outcome, setOutcome] = useState<GenerationOutcome | null>(null);

    const reset = useCallback(() => {
        setStreamContent("");
        setOutcome(null);
    }, []);

    const readAttempt = async (attemptId: string): Promise<SummaryAttempt | null> => {
        try {
            const res = await fetch(`/api/v1/transcription/${audioId}/summary/attempts/${attemptId}`, {
                headers: getAuthHeaders(),
            });
            return res.ok ? ((await res.json()) as SummaryAttempt) : null;
        } catch {
            return null;
        }
    };

    const generateSummary = async (templateId: string, model: string, prompt: string, transcriptText: string, includeSpeakerInfo?: boolean) => {
        setIsStreaming(true);
        setStreamContent("");
        setOutcome(null);

        const transcriptLabel = includeSpeakerInfo
            ? 'Transcript (with speaker labels - each line is prefixed with [SPEAKER_NAME]):'
            : 'Transcript:';
        const combinedContent = `${transcriptLabel}\n${transcriptText}\n\nInstructions:\n${prompt}`;
        let attemptId: string | null = null;
        let streamError: string | null = null;

        try {
            const res = await fetch('/api/v1/summarize', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
                body: JSON.stringify({
                    model: model,
                    content: combinedContent,
                    transcription_id: audioId,
                    template_id: templateId
                }),
            });
            if (!res.ok) {
                setOutcome(outcomeFromBody(await readJSON(res), `Summary request failed (HTTP ${res.status}).`));
                return;
            }
            attemptId = res.headers.get('X-Summary-Attempt-Id');
            if (!res.body) {
                throw new Error('The server did not return a stream.');
            }
            const reader = res.body.getReader();
            const decoder = new TextDecoder();
            while (true) {
                const { done, value } = await reader.read();
                if (done) {
                    const tail = decoder.decode();
                    if (tail) setStreamContent(prev => prev + tail);
                    break;
                }
                const chunk = decoder.decode(value, { stream: true });
                if (chunk) setStreamContent(prev => prev + chunk);
            }
        } catch (e) {
            streamError = e instanceof Error ? e.message : "Summary generation failed.";
        } finally {
            if (attemptId) {
                // The 200 status was sent before generation finished; only the
                // attempt record says whether the text was complete and saved.
                const attempt = await readAttempt(attemptId);
                setOutcome(attempt
                    ? {
                        attempt,
                        error: attempt.status === "completed" ? null : (attempt.detail || "Generation did not complete. Nothing was saved."),
                    }
                    : { attempt: null, error: "Could not confirm whether the summary was saved. Reopen the summary to check." });
            } else if (streamError) {
                setOutcome({ attempt: null, error: streamError });
            }
            setIsStreaming(false);
            queryClient.invalidateQueries({ queryKey: ["summary", audioId] });
        }
    };

    return { generateSummary, isStreaming, streamContent, outcome, reset };
}

// Evidence-linked draft. The server builds the input from the stored
// transcript, checks every reference and returns the attempt outcome.
export function useGroundedSummary(audioId: string) {
    const { getAuthHeaders } = useAuth();
    const queryClient = useQueryClient();
    const [isGenerating, setIsGenerating] = useState(false);
    const [outcome, setOutcome] = useState<GenerationOutcome | null>(null);

    const generate = async (templateId: string, model: string) => {
        setIsGenerating(true);
        setOutcome(null);
        try {
            const res = await fetch(`/api/v1/transcription/${audioId}/summary/grounded`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
                body: JSON.stringify({ model, template_id: templateId }),
            });
            const body = await readJSON(res);
            if (res.status === 201) {
                setOutcome({ attempt: (body?.attempt as SummaryAttempt | undefined) ?? null, error: null });
            } else {
                setOutcome(outcomeFromBody(body, `Generation failed (HTTP ${res.status}).`));
            }
        } catch (e) {
            setOutcome({ attempt: null, error: e instanceof Error ? e.message : "Generation failed." });
        } finally {
            setIsGenerating(false);
            queryClient.invalidateQueries({ queryKey: ["summary", audioId] });
        }
    };

    const reset = useCallback(() => setOutcome(null), []);

    return { generate, isGenerating, outcome, reset };
}
