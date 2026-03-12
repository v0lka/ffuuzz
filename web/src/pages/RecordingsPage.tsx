import { useState, useCallback, useRef, useEffect } from "react";
import { Link } from "react-router-dom";
import { Upload, Download, Trash2, Disc3, X } from "lucide-react";
import { useRecordings, useDeleteRecording } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import { EmptyState } from "@/components/EmptyState";
import { TimeAgo } from "@/components/TimeAgo";
import { Pagination } from "@/components/Pagination";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ImportDialog } from "@/components/ImportDialog";
import { EndpointTree } from "@/components/EndpointTree";
import type { EndpointFilter } from "@/components/EndpointTree";
import { ApiClientError } from "@/api/client";
import { exportRecordings } from "@/api/recordings";

const PAGE_SIZE = 20;
const LS_TREE_WIDTH_KEY = "ffuuzz:tree-width";
const DEFAULT_TREE_WIDTH = 288;
const MIN_TREE_WIDTH = 200;
const MAX_TREE_WIDTH = 600;

function loadTreeWidth(): number {
    try {
        const raw = localStorage.getItem(LS_TREE_WIDTH_KEY);
        if (raw) {
            const n = parseInt(raw, 10);
            if (n >= MIN_TREE_WIDTH && n <= MAX_TREE_WIDTH) return n;
        }
    } catch {
        // ignore
    }
    return DEFAULT_TREE_WIDTH;
}

export default function RecordingsPage() {
    const [offset, setOffset] = useState(0);
    const [host, setHost] = useState("");
    const [importOpen, setImportOpen] = useState(false);
    const [deleteId, setDeleteId] = useState<string | null>(null);
    const [treeFilter, setTreeFilter] = useState<EndpointFilter | null>(null);
    const [treeWidth, setTreeWidth] = useState(loadTreeWidth);
    const [isDragging, setIsDragging] = useState(false);
    const [exporting, setExporting] = useState(false);
    const dragRef = useRef<{ startX: number; startWidth: number } | null>(null);
    const treeWidthRef = useRef(treeWidth);
    treeWidthRef.current = treeWidth;

    const { data, isLoading, error } = useRecordings({
        limit: PAGE_SIZE,
        offset,
        host: host || undefined,
        path_prefix: treeFilter?.pathPrefix || undefined,
    });
    const deleteMutation = useDeleteRecording();

    const handleTreeFilterChange = (filter: EndpointFilter | null) => {
        setTreeFilter(filter);
        setOffset(0);
    };

    // Drag handle logic
    const handleDragStart = useCallback(
        (e: React.MouseEvent) => {
            e.preventDefault();
            dragRef.current = { startX: e.clientX, startWidth: treeWidth };
            setIsDragging(true);
        },
        [treeWidth],
    );

    useEffect(() => {
        if (!isDragging) return;
        const handleMouseMove = (e: MouseEvent) => {
            if (!dragRef.current) return;
            const deltaX = dragRef.current.startX - e.clientX;
            const newWidth = Math.min(
                MAX_TREE_WIDTH,
                Math.max(MIN_TREE_WIDTH, dragRef.current.startWidth + deltaX),
            );
            setTreeWidth(newWidth);
        };
        const handleMouseUp = () => {
            setIsDragging(false);
            dragRef.current = null;
            try {
                localStorage.setItem(LS_TREE_WIDTH_KEY, String(treeWidthRef.current));
            } catch { /* quota or privacy mode */ }
        };
        document.addEventListener("mousemove", handleMouseMove);
        document.addEventListener("mouseup", handleMouseUp);
        document.body.classList.add("select-none");
        return () => {
            document.removeEventListener("mousemove", handleMouseMove);
            document.removeEventListener("mouseup", handleMouseUp);
            document.body.classList.remove("select-none");
        };
    }, [isDragging]);

    const handleExport = useCallback(async () => {
        setExporting(true);
        try {
            await exportRecordings({
                host: host || undefined,
                path_prefix: treeFilter?.pathPrefix || undefined,
            });
        } finally {
            setExporting(false);
        }
    }, [host, treeFilter]);

    return (
        <div className="flex">
            {/* Main content */}
            <div className="flex-1 min-w-0 space-y-4 pr-2">
                <div className="flex items-center justify-between">
                    <h1 className="text-2xl font-bold">Recordings</h1>
                    <div className="flex items-center gap-2">
                        <button
                            className="btn btn-outline btn-sm"
                            onClick={handleExport}
                            disabled={exporting || (!data?.length)}
                        >
                            <Download size={16} />
                            {exporting ? "Exporting..." : "Export"}
                        </button>
                        <button
                            className="btn btn-primary btn-sm"
                            onClick={() => setImportOpen(true)}
                        >
                            <Upload size={16} />
                            Import
                        </button>
                    </div>
                </div>

                {/* Filters */}
                <div className="flex items-center gap-2 flex-wrap">
                    <input
                        type="text"
                        placeholder="Filter by..."
                        className="input input-bordered input-sm w-full max-w-xs"
                        value={host}
                        onChange={(e) => {
                            setHost(e.target.value);
                            setOffset(0);
                        }}
                    />
                    {treeFilter && (
                        <div className="badge badge-primary gap-1">
                            {treeFilter.pathPrefix || treeFilter.scheme + "://" + treeFilter.host + ":" + treeFilter.port}
                            <button
                                onClick={() => handleTreeFilterChange(null)}
                                className="hover:text-primary-content/70"
                            >
                                <X size={12} />
                            </button>
                        </div>
                    )}
                </div>

                {/* Table */}
                {error ? (
                    <ErrorAlert message={error instanceof ApiClientError ? error.message : "Failed to load recordings"} />
                ) : isLoading ? (
                    <LoadingSpinner />
                ) : data && data.length > 0 ? (
                    <>
                        <div className="overflow-x-auto">
                            <table className="table table-sm">
                                <thead>
                                    <tr>
                                        <th>ID</th>
                                        <th>Target</th>
                                        <th>Entries</th>
                                        <th>Created</th>
                                        <th />
                                    </tr>
                                </thead>
                                <tbody>
                                    {data.map((r) => (
                                        <tr key={r.id} className="hover">
                                            <td>
                                                <Link
                                                    to={`/recordings/${r.id}`}
                                                    className="link link-primary font-mono text-xs"
                                                >
                                                    {r.id.slice(0, 8)}
                                                </Link>
                                            </td>
                                            <td className="font-mono text-xs">
                                                {r.target.scheme}://{r.target.host}:{r.target.port}{r.target.path}
                                            </td>
                                            <td>{r.entry_count ?? r.entries?.length ?? 0}</td>
                                            <td>
                                                <TimeAgo date={r.created_at} />
                                            </td>
                                            <td>
                                                <button
                                                    className="btn btn-ghost btn-xs text-error"
                                                    onClick={() => setDeleteId(r.id)}
                                                >
                                                    <Trash2 size={14} />
                                                </button>
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
                        icon={<Disc3 size={48} />}
                        title="No recordings yet"
                        description="Import recorded sessions or start the proxy to capture traffic"
                        action={
                            <button
                                className="btn btn-primary btn-sm"
                                onClick={() => setImportOpen(true)}
                            >
                                <Upload size={16} />
                                Import
                            </button>
                        }
                    />
                )}

                {/* Import dialog */}
                <ImportDialog open={importOpen} onClose={() => setImportOpen(false)} />

                {/* Delete confirmation */}
                {deleteId && (
                    <ConfirmDialog
                        title="Delete Recording"
                        message="This recording will be permanently deleted. This cannot be undone."
                        confirmLabel="Delete"
                        onConfirm={() => {
                            deleteMutation.mutate(deleteId);
                            setDeleteId(null);
                        }}
                        onCancel={() => setDeleteId(null)}
                    />
                )}
            </div>

            {/* Drag handle */}
            <div
                className={`w-1 cursor-col-resize shrink-0 rounded transition-colors ${isDragging ? "bg-primary/40" : "hover:bg-primary/20"}`}
                onMouseDown={handleDragStart}
            />

            {/* Endpoint tree panel */}
            <EndpointTree
                onFilterChange={handleTreeFilterChange}
                activeFilter={treeFilter}
                className="shrink-0"
                style={{ width: treeWidth }}
            />
        </div>
    );
}
