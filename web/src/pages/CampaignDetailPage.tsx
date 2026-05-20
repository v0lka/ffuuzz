import { useState, useEffect } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { ArrowLeft, Play, Square, Wifi, WifiOff, Sparkles, Pencil } from "lucide-react";
import {
    useCampaign,
    useCampaignStats,
    useCampaignConfig,
    useCampaignFindings,
    useStartCampaign,
    useStopCampaign,
    useAnalyzeCampaign,
    useBatchAnalysisProgress,
} from "@/hooks/queries";
import { useCampaignStream } from "@/hooks/useSSE";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { CampaignStatusBadge } from "@/components/StatusBadge";
import { StatsCard } from "@/components/StatsCard";
import { JsonViewer } from "@/components/JsonViewer";
import { formatDateTime } from "@/components/TimeAgo";
import { Pagination } from "@/components/Pagination";
import { FindingsTable } from "@/components/FindingsTable";
import type { CampaignStatus } from "@/types/api";

const PAGE_SIZE = 20;

const BATCH_STORAGE_PREFIX = "ffuuzz:batchAnalysis:";

function getStoredBatchState(campaignId: string) {
    try {
        const raw = sessionStorage.getItem(BATCH_STORAGE_PREFIX + campaignId);
        if (raw) return JSON.parse(raw) as { active: boolean; total: number };
    } catch { /* ignore corrupt entries */ }
    return { active: false, total: 0 };
}

function isActive(status: CampaignStatus) {
    return status === "RUNNING" || status === "STARTING";
}

function canStart(status: CampaignStatus) {
    return ["CREATED", "STOPPED", "FINISHED", "FAILED"].includes(status);
}

export default function CampaignDetailPage() {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const safeId = id ?? "";
    const campaign = useCampaign(safeId);
    const config = useCampaignConfig(safeId);
    const startMutation = useStartCampaign();
    const stopMutation = useStopCampaign();

    const [tab, setTab] = useState<"findings" | "config" | "info">("findings");
    const [findingOffset, setFindingOffset] = useState(0);
    const [batchAnalysis, setBatchAnalysis] = useState<{
        active: boolean;
        total: number;
    }>(() => getStoredBatchState(safeId));

    // Persist batch analysis state to sessionStorage so progress survives navigation.
    useEffect(() => {
        if (batchAnalysis.active) {
            sessionStorage.setItem(
                BATCH_STORAGE_PREFIX + safeId,
                JSON.stringify(batchAnalysis),
            );
        } else {
            sessionStorage.removeItem(BATCH_STORAGE_PREFIX + safeId);
        }
    }, [batchAnalysis, safeId]);

    const analyzeCampaignMutation = useAnalyzeCampaign();
    const batchProgress = useBatchAnalysisProgress(
        safeId,
        batchAnalysis.total,
        batchAnalysis.active,
    );

    // Auto-dismiss batch analysis progress when all findings are analyzed.
    useEffect(() => {
        if (
            batchProgress.data &&
            batchProgress.data.analyzed >= batchProgress.data.total
        ) {
            const timer = setTimeout(() => {
                setBatchAnalysis({ active: false, total: 0 });
            }, 3_000);
            return () => clearTimeout(timer);
        }
    }, [batchProgress.data]);

    const campaignStatus = campaign.data?.status ?? "CREATED";
    const sseEnabled = isActive(campaignStatus);

    const { isConnected } = useCampaignStream(safeId, sseEnabled);

    const stats = useCampaignStats(safeId, !isActive(campaignStatus), isConnected ? false : 5_000);
    const findings = useCampaignFindings(safeId, {
        limit: PAGE_SIZE,
        offset: findingOffset,
    }, isConnected ? false : 2_000);

    if (!id) return <ErrorAlert message="Missing campaign ID" />;
    if (campaign.isLoading) return <LoadingSpinner />;
    if (campaign.error || !campaign.data)
        return <ErrorAlert message="Campaign not found" />;

    const c = campaign.data;

    return (
        <div className="space-y-6">
            <Link to="/campaigns" className="btn btn-ghost btn-sm gap-1">
                <ArrowLeft size={16} />
                Back
            </Link>

            {/* Header */}
            <div className="flex items-center justify-between flex-wrap gap-3">
                <div>
                    <h1 className="text-2xl font-bold">{c.name}</h1>
                    <div className="flex items-center gap-2 mt-1">
                        <CampaignStatusBadge status={c.status} />
                        {sseEnabled && (
                            <span className="flex items-center gap-1 text-xs opacity-60">
                                {isConnected ? (
                                    <>
                                        <Wifi size={12} className="text-success" />
                                        Live
                                    </>
                                ) : (
                                    <>
                                        <WifiOff size={12} className="text-warning" />
                                        Reconnecting
                                    </>
                                )}
                            </span>
                        )}
                    </div>
                </div>
                <div className="flex gap-2">
                    <button
                        className="btn btn-accent btn-sm"
                        disabled={
                            analyzeCampaignMutation.isPending ||
                            batchAnalysis.active
                        }
                        onClick={() => {
                            analyzeCampaignMutation.mutate(safeId, {
                                onSuccess: (data) => {
                                    setBatchAnalysis({
                                        active: true,
                                        total: data.total ?? 0,
                                    });
                                },
                            });
                        }}
                    >
                        {analyzeCampaignMutation.isPending ? (
                            <span className="loading loading-spinner loading-sm" />
                        ) : (
                            <Sparkles size={16} />
                        )}
                        Batch LLM analyze
                    </button>
                    {canStart(c.status) && (
                        <button
                            className="btn btn-outline btn-sm"
                            onClick={() => navigate(`/campaigns/${safeId}/edit`)}
                        >
                            <Pencil size={16} />
                            Edit
                        </button>
                    )}
                    {canStart(c.status) && (
                        <button
                            className="btn btn-success btn-sm"
                            disabled={startMutation.isPending}
                            onClick={() => startMutation.mutate(safeId)}
                        >
                            <Play size={16} />
                            Start
                        </button>
                    )}
                    {isActive(c.status) && (
                        <button
                            className="btn btn-error btn-sm"
                            disabled={stopMutation.isPending}
                            onClick={() => stopMutation.mutate(safeId)}
                        >
                            <Square size={16} />
                            Stop
                        </button>
                    )}
                </div>
            </div>

            {/* Batch LLM analysis progress */}
            {batchAnalysis.active && (
                <div className="space-y-1">
                    <div className="flex items-center justify-between text-sm">
                        <span className="opacity-60">
                            LLM analysis:{" "}
                            {batchProgress.data?.analyzed ?? 0} /{" "}
                            {batchAnalysis.total} findings analyzed
                        </span>
                        {batchProgress.isFetching && (
                            <span className="loading loading-spinner loading-xs" />
                        )}
                    </div>
                    <progress
                        className="progress progress-accent w-full"
                        value={batchProgress.data?.analyzed ?? 0}
                        max={batchAnalysis.total}
                    />
                </div>
            )}

            {/* Stats */}
            {stats.data && (
                <div className="stats stats-vertical sm:stats-horizontal shadow w-full text-sm">
                    <StatsCard label="Tests" value={stats.data.tests_total} />
                    <StatsCard
                        label="Tests/sec"
                        value={stats.data.tests_per_sec.toFixed(1)}
                    />
                    <StatsCard label="Timeouts" value={stats.data.timeouts} />
                    <StatsCard label="5xx Errors" value={stats.data.server_errors} />
                    <StatsCard
                        label="Latency Reg."
                        value={stats.data.latency_regressions}
                    />
                    <StatsCard label="Regex Matches" value={stats.data.regex_matches} />
                </div>
            )}

            {/* Tabs */}
            <div role="tablist" className="tabs tabs-bordered">
                {(["findings", "config", "info"] as const).map((t) => (
                    <button
                        key={t}
                        role="tab"
                        className={`tab ${tab === t ? "tab-active" : ""}`}
                        onClick={() => setTab(t)}
                    >
                        {t.charAt(0).toUpperCase() + t.slice(1)}
                        {t === "findings"
                            ? ` (${c.progress?.findings_total ?? 0})`
                            : ""}
                    </button>
                ))}
            </div>

            {/* Tab content */}
            {tab === "findings" && (
                <div>
                    {findings.isLoading ? (
                        <LoadingSpinner />
                    ) : findings.data && findings.data.length > 0 ? (
                        <>
                            <FindingsTable findings={findings.data} from={`/campaigns/${safeId}`} />
                            <Pagination
                                offset={findingOffset}
                                limit={PAGE_SIZE}
                                hasMore={findings.data.length === PAGE_SIZE}
                                onPageChange={setFindingOffset}
                            />
                        </>
                    ) : (
                        <p className="text-sm opacity-60 p-4">No findings yet</p>
                    )}
                </div>
            )}

            {tab === "config" && (
                <div>
                    {config.isLoading ? (
                        <LoadingSpinner />
                    ) : config.data ? (
                        <JsonViewer data={config.data} />
                    ) : (
                        <p className="text-sm opacity-60">Config not available</p>
                    )}
                </div>
            )}

            {tab === "info" && (
                <div className="card bg-base-200">
                    <div className="card-body p-4 text-sm space-y-2">
                        <p>
                            <span className="opacity-60">ID:</span>{" "}
                            <span className="font-mono">{c.id}</span>
                        </p>
                        <p>
                            <span className="opacity-60">Created:</span>{" "}
                            {formatDateTime(c.created_at)}
                        </p>
                        {c.started_at && (
                            <p>
                                <span className="opacity-60">Started:</span>{" "}
                                {formatDateTime(c.started_at)}
                            </p>
                        )}
                        {c.finished_at && (
                            <p>
                                <span className="opacity-60">Finished:</span>{" "}
                                {formatDateTime(c.finished_at)}
                            </p>
                        )}
                        {c.recording_ids && c.recording_ids.length > 0 && (
                            <div>
                                <span className="opacity-60">Recordings:</span>
                                <ul className="list-disc list-inside ml-2">
                                    {c.recording_ids.map((rid) => (
                                        <li key={rid}>
                                            <Link
                                                to={`/recordings/${rid}`}
                                                className="link link-primary font-mono text-xs"
                                            >
                                                {rid.slice(0, 8)}
                                            </Link>
                                        </li>
                                    ))}
                                </ul>
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
