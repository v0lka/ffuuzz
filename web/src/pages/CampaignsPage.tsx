import { useState } from "react";
import { Link } from "react-router-dom";
import { Plus, Rocket } from "lucide-react";
import { useCampaigns } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { EmptyState } from "@/components/EmptyState";
import { CampaignStatusBadge } from "@/components/StatusBadge";
import { TimeAgo } from "@/components/TimeAgo";
import { Pagination } from "@/components/Pagination";
import { ApiClientError } from "@/api/client";
import type { CampaignStatus } from "@/types/api";

const PAGE_SIZE = 20;
const statusOptions: Array<CampaignStatus | ""> = [
    "",
    "CREATED",
    "STARTING",
    "RUNNING",
    "STOPPING",
    "STOPPED",
    "FINISHED",
    "FAILED",
];

export default function CampaignsPage() {
    const [offset, setOffset] = useState(0);
    const [status, setStatus] = useState<string>("");

    const { data, isLoading, error } = useCampaigns({
        status: status || undefined,
        limit: PAGE_SIZE,
        offset,
    });

    if (isLoading) return <LoadingSpinner />;
    if (error) {
        const msg = error instanceof ApiClientError ? error.message : "Failed to load campaigns";
        return <ErrorAlert message={msg} />;
    }

    return (
        <div className="space-y-4">
            <div className="flex items-center justify-between">
                <h1 className="text-2xl font-bold">Campaigns</h1>
                <Link to="/campaigns/new" className="btn btn-primary btn-sm">
                    <Plus size={16} />
                    New Campaign
                </Link>
            </div>

            {/* Filter */}
            <select
                className="select select-bordered select-sm w-full max-w-xs"
                value={status}
                onChange={(e) => {
                    setStatus(e.target.value);
                    setOffset(0);
                }}
            >
                <option value="">All Statuses</option>
                {statusOptions
                    .filter((s) => s !== "")
                    .map((s) => (
                        <option key={s} value={s}>
                            {s}
                        </option>
                    ))}
            </select>

            {data && data.length > 0 ? (
                <>
                    <div className="overflow-x-auto">
                        <table className="table table-sm">
                            <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>Status</th>
                                    <th>Tests</th>
                                    <th>Findings</th>
                                    <th>Created</th>
                                </tr>
                            </thead>
                            <tbody>
                                {data.map((c) => (
                                    <tr key={c.id} className="hover">
                                        <td>
                                            <Link
                                                to={`/campaigns/${c.id}`}
                                                className="link link-primary"
                                            >
                                                {c.name}
                                            </Link>
                                        </td>
                                        <td>
                                            <CampaignStatusBadge status={c.status} />
                                        </td>
                                        <td>{c.progress?.tests_done ?? 0}</td>
                                        <td>{c.progress?.findings_total ?? 0}</td>
                                        <td>
                                            <TimeAgo date={c.created_at} />
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                    <Pagination
                        offset={offset}
                        limit={PAGE_SIZE}
                        hasMore={data.length === PAGE_SIZE}
                        onPageChange={setOffset}
                    />
                </>
            ) : (
                <EmptyState
                    icon={<Rocket size={48} />}
                    title="No campaigns yet"
                    description="Create a campaign to start fuzzing"
                    action={
                        <Link to="/campaigns/new" className="btn btn-primary btn-sm">
                            <Plus size={16} />
                            New Campaign
                        </Link>
                    }
                />
            )}
        </div>
    );
}
