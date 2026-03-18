import { useState } from "react";
import { AlertTriangle } from "lucide-react";
import { useFindings, useCampaigns } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { EmptyState } from "@/components/EmptyState";
import { FindingsTable } from "@/components/FindingsTable";
import { Pagination } from "@/components/Pagination";
import { ApiClientError } from "@/api/client";

const PAGE_SIZE = 20;

export default function FindingsPage() {
    const [offset, setOffset] = useState(0);
    const [campaignId, setCampaignId] = useState("");
    const [typeFilter, setTypeFilter] = useState("");
    const [statusFilter, setStatusFilter] = useState("");

    const campaigns = useCampaigns({ limit: 100 });
    const { data, isLoading, error } = useFindings({
        campaign_id: campaignId || undefined,
        type: typeFilter || undefined,
        status: statusFilter || undefined,
        limit: PAGE_SIZE,
        offset,
    });

    if (isLoading) return <LoadingSpinner />;
    if (error) {
        const msg = error instanceof ApiClientError ? error.message : "Failed to load findings";
        return <ErrorAlert message={msg} />;
    }

    // Build campaign name lookup
    const campaignNames = new Map<string, string>();
    campaigns.data?.forEach((c) => campaignNames.set(c.id, c.name));

    return (
        <div className="space-y-4">
            <h1 className="text-2xl font-bold">Findings</h1>

            {/* Filters */}
            <div className="flex flex-wrap gap-2">
                <select
                    className="select select-bordered select-sm"
                    value={campaignId}
                    onChange={(e) => {
                        setCampaignId(e.target.value);
                        setOffset(0);
                    }}
                >
                    <option value="">All Campaigns</option>
                    {campaigns.data?.map((c) => (
                        <option key={c.id} value={c.id}>
                            {c.name}
                        </option>
                    ))}
                </select>
                <select
                    className="select select-bordered select-sm"
                    value={typeFilter}
                    onChange={(e) => {
                        setTypeFilter(e.target.value);
                        setOffset(0);
                    }}
                >
                    <option value="">All Types</option>
                    <option value="TIMEOUT">TIMEOUT</option>
                    <option value="SERVER_ERROR">SERVER ERROR</option>
                    <option value="LATENCY_REGRESSION">LATENCY REGRESSION</option>
                    <option value="REGEX_MATCH">REGEX MATCH</option>
                </select>
                <select
                    className="select select-bordered select-sm"
                    value={statusFilter}
                    onChange={(e) => {
                        setStatusFilter(e.target.value);
                        setOffset(0);
                    }}
                >
                    <option value="">All Statuses</option>
                    <option value="UNCONFIRMED">UNCONFIRMED</option>
                    <option value="CONFIRMED">CONFIRMED</option>
                </select>
            </div>

            {data && data.length > 0 ? (
                <>
                    <FindingsTable
                        findings={data}
                        showCampaign={true}
                        campaignNames={campaignNames}
                    />
                    <Pagination
                        offset={offset}
                        limit={PAGE_SIZE}
                        hasMore={data.length === PAGE_SIZE}
                        onPageChange={setOffset}
                    />
                </>
            ) : (
                <EmptyState
                    icon={<AlertTriangle size={48} />}
                    title="No findings"
                    description="Findings will appear here when campaigns detect anomalies"
                />
            )}
        </div>
    );
}
