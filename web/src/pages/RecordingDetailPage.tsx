import { useParams, Link } from "react-router-dom";
import { ArrowLeft } from "lucide-react";
import { useRecording } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { ExchangeViewer } from "@/components/ExchangeViewer";
import { formatDateTime } from "@/components/TimeAgo";

export default function RecordingDetailPage() {
    const { id } = useParams<{ id: string }>();
    const { data, isLoading, error } = useRecording(id ?? "", true);

    if (!id) return <ErrorAlert message="Missing recording ID" />;
    if (isLoading) return <LoadingSpinner />;
    if (error || !data) return <ErrorAlert message="Recording not found" />;

    return (
        <div className="space-y-6">
            <Link to="/recordings" className="btn btn-ghost btn-sm gap-1">
                <ArrowLeft size={16} />
                Back
            </Link>

            <div>
                <h1 className="text-2xl font-bold">Recording</h1>
                <p className="font-mono text-sm opacity-70">{data.id}</p>
            </div>

            {/* Metadata */}
            <div className="card bg-base-200">
                <div className="card-body p-4">
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                        <div>
                            <span className="opacity-60">Target</span>
                            <p className="font-mono">
                                {data.target.scheme}://{data.target.host}:{data.target.port}
                                {data.target.path}
                            </p>
                        </div>
                        <div>
                            <span className="opacity-60">Schema Version</span>
                            <p>{data.schema_version}</p>
                        </div>
                        <div>
                            <span className="opacity-60">Entries</span>
                            <p>{data.entry_count ?? data.entries?.length ?? 0}</p>
                        </div>
                        <div>
                            <span className="opacity-60">Created</span>
                            <p>{formatDateTime(data.created_at)}</p>
                        </div>
                    </div>
                </div>
            </div>

            {/* Entries */}
            <div>
                <h2 className="text-lg font-semibold mb-3">Entries</h2>
                <ExchangeViewer entries={data.entries ?? []} />
            </div>
        </div>
    );
}
