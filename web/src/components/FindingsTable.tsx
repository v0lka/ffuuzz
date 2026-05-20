import { Link } from "react-router-dom";
import { FindingStatusBadge, findingTypeColors } from "@/components/StatusBadge";
import { TimeAgo } from "@/components/TimeAgo";
import { truncateMiddle, formatMutationType, findingTypeLabels } from "@/utils/formatting";
import type { Finding } from "@/types/api";

interface FindingsTableProps {
    findings: Finding[];
    showCampaign?: boolean;
    campaignNames?: Map<string, string>;
    /** Path to return to when viewing a finding detail. Passed as location state. */
    from?: string;
}

/**
 * Shared findings table component used by both FindingsPage and CampaignDetailPage.
 * Columns: Type, Status, Campaign, Method, Endpoint, Mutation, Payload, When
 *
 * Uses auto table layout (no table-fixed) so shrink-to-content columns work naturally.
 * - Shrink-to-content columns (Type, Status, Campaign, Method, Mutation, When):
 *   whitespace-nowrap on both <th> and <td> → column is as wide as its widest cell.
 * - Elastic columns (Endpoint, Payload): w-[1%] + whitespace-nowrap on <th> sets a
 *   minimal base; the inner <div> with max-w-0 overflow-hidden text-ellipsis clips
 *   the content and the column expands to fill available space via table auto layout.
 */
export function FindingsTable({ findings, showCampaign = false, campaignNames, from }: FindingsTableProps) {
    return (
        <table className="table table-sm w-full">
            <thead>
                <tr>
                    <th className="whitespace-nowrap">Type</th>
                    <th className="whitespace-nowrap">Status</th>
                    {showCampaign && <th className="whitespace-nowrap">Campaign</th>}
                    <th className="whitespace-nowrap">Method</th>
                    <th className="w-[30%]">Endpoint</th>
                    <th className="whitespace-nowrap">Mutation</th>
                    <th className="w-[30%]">Payload</th>
                    <th className="whitespace-nowrap">When</th>
                </tr>
            </thead>
            <tbody>
                {findings.map((f) => (
                    <tr key={f.id} className="hover">
                        <td className="whitespace-nowrap">
                            <Link
                                to={`/findings/${f.id}`}
                                state={from ? { from } : undefined}
                                className="link link-primary"
                            >
                                <span className={`badge badge-sm ${findingTypeColors[f.type]}`}>
                                    {findingTypeLabels[f.type]}
                                </span>
                            </Link>
                        </td>
                        <td className="whitespace-nowrap">
                            {f.llm_analysis ? (
                                <span
                                    className={`badge badge-sm ${f.llm_analysis.classification.toUpperCase() === "NO VULNERABILITY" ? "badge-ghost" : "badge-success"}`}
                                    title={`LLM triage: ${f.llm_analysis.classification.toUpperCase()}`}
                                >
                                    {f.llm_analysis.classification.toUpperCase()}{" "}
                                    <span className="opacity-60 text-[10px]">LLM</span>
                                </span>
                            ) : (
                                <FindingStatusBadge status={f.status} />
                            )}
                        </td>
                        {showCampaign && (
                            <td className="whitespace-nowrap max-w-[8rem] overflow-hidden text-ellipsis">
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
                        <td className="w-[30%] max-w-0">
                            <div className="font-mono text-xs overflow-hidden whitespace-nowrap text-ellipsis" title={f.endpoint}>
                                {f.endpoint}
                            </div>
                        </td>
                        <td className="font-mono text-xs whitespace-nowrap" title={f.mutation_type}>
                            {truncateMiddle(formatMutationType(f.mutation_type), 18)}
                        </td>
                        <td className="w-[30%] max-w-0">
                            <div className="font-mono text-xs overflow-hidden whitespace-nowrap text-ellipsis" title={f.mutation_payload || "-"}>
                                {f.mutation_payload || "-"}
                            </div>
                        </td>
                        <td className="whitespace-nowrap">
                            <TimeAgo date={f.created_at} />
                        </td>
                    </tr>
                ))}
            </tbody>
        </table>
    );
}
