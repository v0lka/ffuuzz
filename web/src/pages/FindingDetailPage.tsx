import { useParams, Link, useLocation } from "react-router-dom";
import { ArrowLeft, RefreshCw, Sparkles } from "lucide-react";
import {
    useFinding,
    useFindingArtifact,
    useReproduceFinding,
    useAnalyzeFinding,
} from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { FindingTypeBadge, FindingStatusBadge, MethodBadge } from "@/components/StatusBadge";
import { ExchangeViewer } from "@/components/ExchangeViewer";
import { formatDateTime } from "@/components/TimeAgo";

export default function FindingDetailPage() {
    const { id } = useParams<{ id: string }>();
    const location = useLocation();
    const safeId = id ?? "";
    const backTo = (location.state as { from?: string } | null)?.from ?? "/findings";
    const { data, isLoading, error } = useFinding(safeId);
    const artifact = useFindingArtifact(safeId, !!data?.artifact_id);
    const reproduceMutation = useReproduceFinding();
    const analyzeMutation = useAnalyzeFinding();

    if (!id) return <ErrorAlert message="Missing finding ID" />;
    if (isLoading) return <LoadingSpinner />;
    if (error || !data) return <ErrorAlert message="Finding not found" />;

    const f = data;
    const isReproducing =
        f.reproduce_status === "ENQUEUED" || f.reproduce_status === "RUNNING";

    return (
        <div className="space-y-6">
            <Link to={backTo} className="btn btn-ghost btn-sm gap-1">
                <ArrowLeft size={16} />
                Back
            </Link>

            {/* Header */}
            <div>
                <div className="flex items-center gap-3 flex-wrap">
                    <FindingTypeBadge type={f.type} />
                    <FindingStatusBadge status={f.status} />
                    <MethodBadge method={f.method} />
                    <span className="font-mono text-sm">{f.endpoint}</span>
                </div>
                <p className="text-xs opacity-50 mt-1 font-mono">{f.id}</p>
            </div>

            {/* Details card */}
            <div className="card bg-base-200">
                <div className="card-body p-4 text-sm space-y-2">
                    <h3 className="font-semibold">Details</h3>
                    {f.type === "TIMEOUT" && (
                        <p>
                            Timeout threshold: {f.details.timeout_ms}ms
                        </p>
                    )}
                    {f.type === "LATENCY_REGRESSION" && (
                        <>
                            <p>Baseline: {f.details.baseline_ms}ms</p>
                            <p>Observed: {f.details.observed_ms}ms</p>
                        </>
                    )}
                    {f.type === "SERVER_ERROR" && (
                        <p>HTTP Status: {f.details.http_status}</p>
                    )}
                    {f.type === "REGEX_MATCH" && (
                        <p>Matched regex pattern in response</p>
                    )}
                </div>
            </div>

            {/* Metadata */}
            <div className="card bg-base-200">
                <div className="card-body p-4 text-sm space-y-2">
                    <h3 className="font-semibold">Metadata</h3>
                    <p>
                        <span className="opacity-60">Campaign:</span>{" "}
                        <Link
                            to={`/campaigns/${f.campaign_id}`}
                            className="link link-primary font-mono text-xs"
                        >
                            {f.campaign_id.slice(0, 8)}
                        </Link>
                    </p>
                    {f.seed_recording_id && (
                        <p>
                            <span className="opacity-60">Seed Recording:</span>{" "}
                            <Link
                                to={`/recordings/${f.seed_recording_id}`}
                                className="link link-primary font-mono text-xs"
                            >
                                {f.seed_recording_id.slice(0, 8)}
                            </Link>
                        </p>
                    )}
                    <p>
                        <span className="opacity-60">Signature:</span>{" "}
                        <span className="font-mono text-xs">{f.signature}</span>
                    </p>
                    <p>
                        <span className="opacity-60">Minimized:</span>{" "}
                        {f.minimized ? "Yes" : "No"}
                    </p>
                    <p>
                        <span className="opacity-60">Created:</span>{" "}
                        {formatDateTime(f.created_at)}
                    </p>
                    {f.confirmed_at && (
                        <p>
                            <span className="opacity-60">Confirmed:</span>{" "}
                            {formatDateTime(f.confirmed_at)}
                        </p>
                    )}
                </div>
            </div>

            {/* Request / Response */}
            {f.artifact_id && (
                <div>
                    <h3 className="font-semibold mb-3">Request / Response</h3>
                    {artifact.isLoading ? (
                        <LoadingSpinner />
                    ) : artifact.data?.session?.entries ? (
                        <ExchangeViewer entries={artifact.data.session.entries} />
                    ) : (
                        <p className="text-sm opacity-60">Artifact not available</p>
                    )}
                </div>
            )}

            {/* LLM Analysis results */}
            {f.llm_analysis && (
                <div className="card bg-base-200">
                    <div className="card-body p-4 text-sm space-y-3">
                        <h3 className="font-semibold">LLM Analysis</h3>
                        <div className="flex items-center gap-2 flex-wrap">
                            <span className="badge badge-primary">
                                {f.llm_analysis.classification}
                            </span>
                            <span className="badge badge-warning">
                                {f.llm_analysis.severity}
                            </span>
                            <span className="badge badge-ghost">
                                Confidence: {(f.llm_analysis.confidence * 100).toFixed(0)}%
                            </span>
                        </div>
                        {f.llm_analysis.exploitability && (
                            <div>
                                <span className="opacity-60">Exploitability:</span>
                                <p className="mt-1">{f.llm_analysis.exploitability}</p>
                            </div>
                        )}
                        {f.llm_analysis.remediation && (
                            <div>
                                <span className="opacity-60">Remediation:</span>
                                <p className="mt-1">{f.llm_analysis.remediation}</p>
                            </div>
                        )}
                        {f.llm_analysis.description && (
                            <div>
                                <span className="opacity-60">Description:</span>
                                <p className="mt-1">{f.llm_analysis.description}</p>
                            </div>
                        )}
                        <div className="text-xs opacity-50">
                            Analyzed: {formatDateTime(f.llm_analysis.analyzed_at)}
                            {f.llm_analysis.model_used && (
                                <> &middot; Model: {f.llm_analysis.model_used}</>
                            )}
                        </div>
                    </div>
                </div>
            )}

            {/* Actions */}
            <div className="flex items-center gap-3 flex-wrap">
                <button
                    className="btn btn-outline btn-sm"
                    disabled={isReproducing || reproduceMutation.isPending}
                    onClick={() => reproduceMutation.mutate({ id: safeId, runs: 3 })}
                >
                    <RefreshCw size={16} />
                    Reproduce (3 runs)
                </button>
                <button
                    className="btn btn-outline btn-sm btn-accent"
                    disabled={analyzeMutation.isPending}
                    onClick={() => analyzeMutation.mutate(safeId)}
                >
                    {analyzeMutation.isPending ? (
                        <span className="loading loading-spinner loading-sm" />
                    ) : (
                        <Sparkles size={16} />
                    )}
                    Analyze with LLM
                </button>
                {f.reproduce_status && (
                    <span className="badge badge-sm badge-info">
                        {f.reproduce_status}
                    </span>
                )}
                {reproduceMutation.isSuccess && (
                    <span className="text-success text-sm">Enqueued</span>
                )}
            </div>
        </div>
    );
}
