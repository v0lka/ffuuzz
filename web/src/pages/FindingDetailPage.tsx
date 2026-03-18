import { useParams, Link } from "react-router-dom";
import { ArrowLeft, RefreshCw } from "lucide-react";
import {
    useFinding,
    useFindingArtifact,
    useReproduceFinding,
} from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { FindingTypeBadge, FindingStatusBadge, MethodBadge } from "@/components/StatusBadge";
import { ExchangeViewer } from "@/components/ExchangeViewer";
import { formatDateTime } from "@/components/TimeAgo";

export default function FindingDetailPage() {
    const { id } = useParams<{ id: string }>();
    const safeId = id ?? "";
    const { data, isLoading, error } = useFinding(safeId);
    const artifact = useFindingArtifact(safeId, !!data?.artifact_id);
    const reproduceMutation = useReproduceFinding();

    if (!id) return <ErrorAlert message="Missing finding ID" />;
    if (isLoading) return <LoadingSpinner />;
    if (error || !data) return <ErrorAlert message="Finding not found" />;

    const f = data;
    const isReproducing =
        f.reproduce_status === "ENQUEUED" || f.reproduce_status === "RUNNING";

    return (
        <div className="space-y-6">
            <Link to="/findings" className="btn btn-ghost btn-sm gap-1">
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

            {/* Reproduce */}
            <div className="flex items-center gap-3">
                <button
                    className="btn btn-outline btn-sm"
                    disabled={isReproducing || reproduceMutation.isPending}
                    onClick={() => reproduceMutation.mutate({ id: safeId, runs: 3 })}
                >
                    <RefreshCw size={16} />
                    Reproduce (3 runs)
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
