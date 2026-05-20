import { Link } from "react-router-dom";
import {
    Disc3,
    Rocket,
    AlertTriangle,
    FlaskConical,
} from "lucide-react";
import { useHealth, useRecordings, useCampaigns, useFindings } from "@/hooks/queries";
import { StatsCard } from "@/components/StatsCard";
import { CampaignStatusBadge } from "@/components/StatusBadge";
import { TimeAgo } from "@/components/TimeAgo";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { FindingsTable } from "@/components/FindingsTable";

export default function DashboardPage() {
    const health = useHealth();
    const recordings = useRecordings({ limit: 10000 });
    const activeCampaigns = useCampaigns({ status: "RUNNING", limit: 10 });
    const allCampaigns = useCampaigns({ limit: 10000 });
    const recentFindings = useFindings({ limit: 10 });

    if (health.isLoading) return <LoadingSpinner />;

    return (
        <div className="space-y-6">
            <h1 className="text-2xl font-bold">Dashboard</h1>

            {/* Summary cards */}
            <div className="stats stats-vertical sm:stats-horizontal shadow w-full">
                <StatsCard
                    icon={<Disc3 size={24} />}
                    label="Recordings"
                    value={recordings.data?.length ?? 0}
                />
                <StatsCard
                    icon={<Rocket size={24} />}
                    label="Active Campaigns"
                    value={activeCampaigns.data?.length ?? 0}
                />
                <StatsCard
                    icon={<FlaskConical size={24} />}
                    label="Total Campaigns"
                    value={allCampaigns.data?.length ?? 0}
                />
                <StatsCard
                    icon={<AlertTriangle size={24} />}
                    label="Recent Findings"
                    value={recentFindings.data?.length ?? 0}
                />
            </div>

            {/* Active campaigns */}
            <div>
                <h2 className="text-lg font-semibold mb-3">Active Campaigns</h2>
                {activeCampaigns.data && activeCampaigns.data.length > 0 ? (
                    <div className="overflow-x-auto">
                        <table className="table table-sm">
                            <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>Status</th>
                                    <th>Tests</th>
                                    <th>Findings</th>
                                    <th>Started</th>
                                </tr>
                            </thead>
                            <tbody>
                                {activeCampaigns.data.map((c) => (
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
                                            {c.started_at ? (
                                                <TimeAgo date={c.started_at} />
                                            ) : (
                                                "-"
                                            )}
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                ) : (
                    <p className="text-sm opacity-60">No active campaigns</p>
                )}
            </div>

            {/* Recent findings */}
            <div>
                <h2 className="text-lg font-semibold mb-3">Recent Findings</h2>
                {recentFindings.data && recentFindings.data.length > 0 ? (
                    <FindingsTable findings={recentFindings.data} from="/" />
                ) : (
                    <p className="text-sm opacity-60">No findings yet</p>
                )}
            </div>
        </div>
    );
}
