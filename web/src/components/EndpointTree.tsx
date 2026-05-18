import { useState, useEffect, useCallback } from "react";
import {
    ChevronRight,
    ChevronDown,
    Globe,
    Folder,
    FileText,
    Trash2,
    FolderPlus,
} from "lucide-react";
import {
    useRecordingsTree,
    useDeleteRecordingsByPrefix,
    useCampaigns,
    useAddRecordingsToCampaign,
} from "@/hooks/queries";
import type { TreePathNode, TreeOrigin, Campaign, CampaignStatus } from "@/types/api";
import { ConfirmDialog } from "@/components/ConfirmDialog";

export interface EndpointFilter {
    scheme: string;
    host: string;
    port: number;
    pathPrefix: string;
}

interface EndpointTreeProps {
    onFilterChange: (filter: EndpointFilter | null) => void;
    activeFilter: EndpointFilter | null;
    className?: string;
    style?: React.CSSProperties;
}

interface ContextMenuState {
    x: number;
    y: number;
    scheme: string;
    host: string;
    port: number;
    pathPrefix: string;
    label: string;
}

const LS_EXPANDED_KEY = "ffuuzz:tree-expanded";

function loadExpanded(): Set<string> {
    try {
        const raw = localStorage.getItem(LS_EXPANDED_KEY);
        if (raw) {
            const arr = JSON.parse(raw) as string[];
            return new Set(arr);
        }
    } catch {
        // ignore
    }
    return new Set();
}

const INACTIVE_STATUSES: Set<CampaignStatus> = new Set([
    "CREATED",
    "STOPPED",
    "FINISHED",
    "FAILED",
    "STOPPING",
]);

export function EndpointTree({
    onFilterChange,
    activeFilter,
    className,
    style,
}: EndpointTreeProps) {
    const { data: tree } = useRecordingsTree();
    const deleteMutation = useDeleteRecordingsByPrefix();
    const [expanded, setExpanded] = useState<Set<string>>(loadExpanded);
    const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(
        null,
    );
    const [confirmDelete, setConfirmDelete] = useState<ContextMenuState | null>(
        null,
    );
    const [campaignSubmenu, setCampaignSubmenu] = useState(false);

    // Campaigns for "Add to campaign" context menu
    const { data: allCampaigns } = useCampaigns();
    const addRecordingsMutation = useAddRecordingsToCampaign();
    const inactiveCampaigns = (allCampaigns ?? []).filter((c: Campaign) =>
        INACTIVE_STATUSES.has(c.status),
    );

    // Persist expanded state to localStorage
    useEffect(() => {
        try {
            localStorage.setItem(LS_EXPANDED_KEY, JSON.stringify([...expanded]));
        } catch { /* quota or privacy mode */ }
    }, [expanded]);

    // Close context menu on click-away
    useEffect(() => {
        if (!contextMenu) return;
        const handler = () => {
            setContextMenu(null);
            setCampaignSubmenu(false);
        };
        document.addEventListener("click", handler);
        return () => document.removeEventListener("click", handler);
    }, [contextMenu]);

    const toggleExpand = useCallback((key: string) => {
        setExpanded((prev) => {
            const next = new Set(prev);
            if (next.has(key)) {
                next.delete(key);
            } else {
                next.add(key);
            }
            return next;
        });
    }, []);

    const handleLeftClick = (filter: EndpointFilter) => {
        if (
            activeFilter &&
            activeFilter.scheme === filter.scheme &&
            activeFilter.host === filter.host &&
            activeFilter.port === filter.port &&
            activeFilter.pathPrefix === filter.pathPrefix
        ) {
            onFilterChange(null);
        } else {
            onFilterChange(filter);
        }
    };

    const handleRightClick = (e: React.MouseEvent, ctx: ContextMenuState) => {
        e.preventDefault();
        setContextMenu(ctx);
        setCampaignSubmenu(false);
    };

    const handleDelete = () => {
        if (!confirmDelete) return;
        deleteMutation.mutate({
            scheme: confirmDelete.scheme,
            host: confirmDelete.host,
            port: confirmDelete.port,
            path_prefix: confirmDelete.pathPrefix || undefined,
        });
        setConfirmDelete(null);
        onFilterChange(null);
    };

    const handleAddToCampaign = (campaignId: string) => {
        if (!contextMenu) return;
        addRecordingsMutation.mutate({
            campaignId,
            filter: {
                scheme: contextMenu.scheme,
                host: contextMenu.host,
                port: contextMenu.port,
                path_prefix: contextMenu.pathPrefix || undefined,
            },
        });
        setContextMenu(null);
        setCampaignSubmenu(false);
    };

    const isActive = (filter: EndpointFilter) =>
        activeFilter !== null &&
        activeFilter.scheme === filter.scheme &&
        activeFilter.host === filter.host &&
        activeFilter.port === filter.port &&
        activeFilter.pathPrefix === filter.pathPrefix;

    return (
        <div
            className={`border-l border-base-300 pl-4 ${className ?? ""}`}
            style={style}
        >
            <div className="flex items-center justify-between mb-2">
                <h3 className="text-sm font-semibold">Endpoints</h3>
            </div>

            {!tree || tree.length === 0 ? (
                <p className="text-xs text-base-content/50">No endpoints</p>
            ) : (
                <div className="space-y-1 text-xs overflow-y-auto max-h-[calc(100vh-12rem)]">
                    {tree.map((origin) => (
                        <OriginNode
                            key={origin.origin}
                            origin={origin}
                            expanded={expanded}
                            toggleExpand={toggleExpand}
                            onLeftClick={handleLeftClick}
                            onRightClick={handleRightClick}
                            isActive={isActive}
                        />
                    ))}
                </div>
            )}

            {/* Context menu */}
            {contextMenu && (
                <div
                    className="fixed z-50 bg-base-100 shadow-lg border border-base-300 rounded-lg py-1 min-w-36"
                    style={{ left: contextMenu.x, top: contextMenu.y }}
                >
                    {/* Add to campaign */}
                    <div
                        className="relative"
                        onMouseEnter={() => setCampaignSubmenu(true)}
                        onMouseLeave={() => setCampaignSubmenu(false)}
                    >
                        <button
                            className="w-full text-left px-3 py-1.5 text-sm hover:bg-base-200 flex items-center gap-2"
                            onClick={(e) => {
                                e.stopPropagation();
                                setCampaignSubmenu((prev) => !prev);
                            }}
                        >
                            <FolderPlus size={14} />
                            Add to campaign
                            <ChevronRight size={12} className="ml-auto" />
                        </button>
                        {campaignSubmenu && (
                            <div
                                className="absolute z-50 bg-base-100 shadow-lg border border-base-300 rounded-lg py-1 min-w-40 max-h-60 overflow-y-auto"
                                style={{ right: "100%", top: 0 }}
                            >
                                {inactiveCampaigns.length === 0 ? (
                                    <span className="px-3 py-1.5 text-sm text-base-content/50 block">
                                        No campaigns available
                                    </span>
                                ) : (
                                    inactiveCampaigns.map((c: Campaign) => (
                                        <button
                                            key={c.id}
                                            className="w-full text-left px-3 py-1.5 text-sm hover:bg-base-200 truncate"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                handleAddToCampaign(c.id);
                                            }}
                                        >
                                            {c.name}
                                        </button>
                                    ))
                                )}
                            </div>
                        )}
                    </div>
                    {/* Delete */}
                    <button
                        className="w-full text-left px-3 py-1.5 text-sm text-error hover:bg-base-200 flex items-center gap-2"
                        onClick={(e) => {
                            e.stopPropagation();
                            setConfirmDelete(contextMenu);
                            setContextMenu(null);
                        }}
                    >
                        <Trash2 size={14} />
                        Delete
                    </button>
                </div>
            )}

            {/* Confirm delete dialog */}
            {confirmDelete && (
                <ConfirmDialog
                    title="Delete Recordings"
                    message={`Delete all recordings for "${confirmDelete.label}"? This cannot be undone.`}
                    confirmLabel="Delete"
                    onConfirm={handleDelete}
                    onCancel={() => setConfirmDelete(null)}
                />
            )}
        </div>
    );
}

function OriginNode({
    origin,
    expanded,
    toggleExpand,
    onLeftClick,
    onRightClick,
    isActive,
}: {
    origin: TreeOrigin;
    expanded: Set<string>;
    toggleExpand: (key: string) => void;
    onLeftClick: (filter: EndpointFilter) => void;
    onRightClick: (e: React.MouseEvent, ctx: ContextMenuState) => void;
    isActive: (filter: EndpointFilter) => boolean;
}) {
    const key = origin.origin;
    const isExpanded = expanded.has(key);
    const hasPaths = (origin.paths?.length ?? 0) > 0;
    const filter: EndpointFilter = {
        scheme: origin.scheme,
        host: origin.host,
        port: origin.port,
        pathPrefix: "",
    };

    return (
        <div>
            <div
                className={`flex items-center gap-1 py-0.5 px-1 rounded cursor-pointer select-none hover:bg-base-200 ${isActive(filter) ? "bg-base-300 font-semibold" : ""
                    }`}
                onClick={() => {
                    onLeftClick(filter);
                }}
                onContextMenu={(e) =>
                    onRightClick(e, {
                        x: e.clientX,
                        y: e.clientY,
                        ...filter,
                        label: origin.origin,
                    })
                }
            >
                {hasPaths ? (
                    <span
                        role="button"
                        className="p-0.5 rounded hover:bg-base-300"
                        onClick={(e) => {
                            e.stopPropagation();
                            toggleExpand(key);
                        }}
                    >
                        {isExpanded ? (
                            <ChevronDown size={12} />
                        ) : (
                            <ChevronRight size={12} />
                        )}
                    </span>
                ) : (
                    <span className="w-3" />
                )}
                <Globe size={12} className="shrink-0 text-primary" />
                <span className="truncate font-mono">{origin.origin}</span>
                <span className="badge badge-xs badge-ghost ml-auto">
                    {origin.recording_count}
                </span>
            </div>
            {isExpanded && hasPaths && (
                <div className="ml-3">
                    {origin.paths.map((node) => (
                        <PathNode
                            key={node.full_path}
                            node={node}
                            depth={1}
                            origin={origin}
                            expanded={expanded}
                            toggleExpand={toggleExpand}
                            onLeftClick={onLeftClick}
                            onRightClick={onRightClick}
                            isActive={isActive}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

function PathNode({
    node,
    depth,
    origin,
    expanded,
    toggleExpand,
    onLeftClick,
    onRightClick,
    isActive,
}: {
    node: TreePathNode;
    depth: number;
    origin: TreeOrigin;
    expanded: Set<string>;
    toggleExpand: (key: string) => void;
    onLeftClick: (filter: EndpointFilter) => void;
    onRightClick: (e: React.MouseEvent, ctx: ContextMenuState) => void;
    isActive: (filter: EndpointFilter) => boolean;
}) {
    const key = `${origin.origin}${node.full_path}`;
    const isExpanded = expanded.has(key);
    const hasChildren = (node.children?.length ?? 0) > 0;
    const filter: EndpointFilter = {
        scheme: origin.scheme,
        host: origin.host,
        port: origin.port,
        pathPrefix: node.full_path,
    };
    const Icon = hasChildren ? Folder : FileText;

    return (
        <div>
            <div
                className={`flex items-center gap-1 py-0.5 px-1 rounded cursor-pointer select-none hover:bg-base-200 ${isActive(filter) ? "bg-base-300 font-semibold" : ""
                    }`}
                style={{ paddingLeft: `${depth * 0.5}rem` }}
                onClick={() => {
                    onLeftClick(filter);
                }}
                onContextMenu={(e) =>
                    onRightClick(e, {
                        x: e.clientX,
                        y: e.clientY,
                        ...filter,
                        label: node.full_path,
                    })
                }
            >
                {hasChildren ? (
                    <span
                        role="button"
                        className="p-0.5 rounded hover:bg-base-300"
                        onClick={(e) => {
                            e.stopPropagation();
                            toggleExpand(key);
                        }}
                    >
                        {isExpanded ? (
                            <ChevronDown size={12} />
                        ) : (
                            <ChevronRight size={12} />
                        )}
                    </span>
                ) : (
                    <span className="w-3" />
                )}
                <Icon size={12} className="shrink-0 text-base-content/60" />
                <span className="truncate font-mono">{node.segment}</span>
                {node.recording_count > 0 && (
                    <span className="badge badge-xs badge-ghost ml-auto">
                        {node.recording_count}
                    </span>
                )}
            </div>
            {isExpanded && hasChildren && (
                <div>
                    {node.children.map((child) => (
                        <PathNode
                            key={child.full_path}
                            node={child}
                            depth={depth + 1}
                            origin={origin}
                            expanded={expanded}
                            toggleExpand={toggleExpand}
                            onLeftClick={onLeftClick}
                            onRightClick={onRightClick}
                            isActive={isActive}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}
