import { Link } from "react-router-dom";
import { FindingStatusBadge, TruncatedEndpoint, findingTypeColors } from "@/components/StatusBadge";
import { TimeAgo } from "@/components/TimeAgo";
import type { Finding, FindingType } from "@/types/api";

interface FindingsTableProps {
    findings: Finding[];
    showCampaign?: boolean;
    campaignNames?: Map<string, string>;
}

/**
 * Truncates a string in the middle with ellipsis if it exceeds maxLen.
 */
function truncateMiddle(str: string, maxLen: number): string {
    if (str.length <= maxLen) return str;
    const half = Math.floor((maxLen - 1) / 2);
    return str.slice(0, half) + "…" + str.slice(-half);
}

/**
 * Truncates a mutation type string for display.
 * Shows only the last part after the colon (e.g., "json:string_mutation" -> "string_mutation").
 */
function formatMutationType(mutationType: string | undefined): string {
    if (!mutationType) return "-";
    const parts = mutationType.split(":");
    if (parts.length > 1) {
        return parts[parts.length - 1] ?? mutationType;
    }
    return mutationType;
}

/**
 * Maps FindingType to a short single-word display label.
 */
const findingTypeLabels: Record<FindingType, string> = {
    TIMEOUT: "TIMEOUT",
    SERVER_ERROR: "5xx",
    LATENCY_REGRESSION: "LATENCY",
    REGEX_MATCH: "REGEX",
};

/**
 * Shared findings table component used by both FindingsPage and CampaignDetailPage.
 * Columns: Type, Status, Method, Endpoint, Mutation, Payload, When
 * - Type, Status, Campaign, Method, Mutation, When: fit to content
 * - Endpoint, Payload: elastic (share remaining space)
 */
export function FindingsTable({ findings, showCampaign = false, campaignNames }: FindingsTableProps) {
    return (
        <div className="overflow-x-auto">
            <table className="table table-sm w-full">
                <thead>
                    <tr>
                        <th>Type</th>
                        <th>Status</th>
                        {showCampaign && <th>Campaign</th>}
                        <th>Method</th>
                        <th className="w-full">Endpoint</th>
                        <th>Mutation</th>
                        <th className="w-full">Payload</th>
                        <th>When</th>
                    </tr>
                </thead>
                <tbody>
                    {findings.map((f) => (
                        <tr key={f.id} className="hover">
                            <td>
                                <Link
                                    to={`/findings/${f.id}`}
                                    className="link link-primary"
                                >
                                    <span className={`badge badge-sm ${findingTypeColors[f.type]}`}>
                                        {findingTypeLabels[f.type]}
                                    </span>
                                </Link>
                            </td>
                            <td>
                                <FindingStatusBadge status={f.status} />
                            </td>
                            {showCampaign && (
                                <td className="whitespace-nowrap">
                                    <Link
                                        to={`/campaigns/${f.campaign_id}`}
                                        className="link link-primary text-xs"
                                    >
                                        {campaignNames?.get(f.campaign_id) ??
                                            f.campaign_id.slice(0, 8)}
                                    </Link>
                                </td>
                            )}
                            <td className="font-mono text-xs whitespace-nowrap">{f.method}</td>
                            <td className="min-w-0">
                                <TruncatedEndpoint value={f.endpoint} />
                            </td>
                            <td className="font-mono text-xs whitespace-nowrap" title={f.mutation_type}>
                                {truncateMiddle(formatMutationType(f.mutation_type), 18)}
                            </td>
                            <td className="min-w-0">
                                <TruncatedEndpoint value={f.mutation_payload || "-"} />
                            </td>
                            <td className="whitespace-nowrap">
                                <TimeAgo date={f.created_at} />
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
