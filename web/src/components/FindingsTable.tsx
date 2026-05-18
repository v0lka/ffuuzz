import { Link } from "react-router-dom";
import { FindingStatusBadge, TruncatedEndpoint, findingTypeColors } from "@/components/StatusBadge";
import { TimeAgo } from "@/components/TimeAgo";
import { truncateMiddle, formatMutationType, findingTypeLabels } from "@/utils/formatting";
import type { Finding } from "@/types/api";

interface FindingsTableProps {
    findings: Finding[];
    showCampaign?: boolean;
    campaignNames?: Map<string, string>;
}

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
