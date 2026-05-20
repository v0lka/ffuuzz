import { useState } from "react";
import type { Exchange } from "@/types/api";
import { MethodBadge } from "@/components/StatusBadge";
import { HeadersTable } from "@/components/HeadersTable";
import { BodyViewer } from "@/components/BodyViewer";

interface ExchangeViewerProps {
    entries: Exchange[];
}

export function ExchangeViewer({ entries }: ExchangeViewerProps) {
    const [expanded, setExpanded] = useState<Set<number>>(new Set());

    const toggle = (idx: number) => {
        setExpanded((prev) => {
            const next = new Set(prev);
            if (next.has(idx)) next.delete(idx);
            else next.add(idx);
            return next;
        });
    };

    if (!entries || entries.length === 0) {
        return (
            <p className="text-sm opacity-60">No entries available.</p>
        );
    }

    return (
        <div className="space-y-2">
            {entries.map((ex, idx) => (
                <div key={ex.request_id} className="collapse collapse-arrow bg-base-200">
                    <input
                        type="checkbox"
                        checked={expanded.has(idx)}
                        onChange={() => toggle(idx)}
                    />
                    <div className="collapse-title flex items-center gap-3 text-sm">
                        <span className="opacity-50 w-6">#{idx + 1}</span>
                        <MethodBadge method={ex.request.method} />
                        <span className="font-mono">
                            {ex.request.path}
                            {ex.request.query && (
                                <span className="opacity-70">?{ex.request.query}</span>
                            )}
                        </span>
                        <span
                            className={`badge badge-xs ${ex.response.status >= 400
                                ? "badge-error"
                                : ex.response.status >= 300
                                    ? "badge-warning"
                                    : "badge-success"
                                }`}
                        >
                            {ex.response.status}
                        </span>
                        <span className="opacity-50">{ex.duration_ms}ms</span>
                    </div>
                    {expanded.has(idx) && (
                        <div className="collapse-content">
                            <div className="grid md:grid-cols-2 gap-4 pt-2">
                                <div className="min-w-0 overflow-hidden">
                                    <h4 className="text-xs font-bold mb-2">
                                        Request Headers
                                    </h4>
                                    <HeadersTable headers={ex.request.headers} />
                                    <h4 className="text-xs font-bold mt-3 mb-2">
                                        Request Body
                                    </h4>
                                    <BodyViewer
                                        bodyB64={ex.request.body_b64}
                                        truncated={ex.request.body_truncated}
                                    />
                                </div>
                                <div className="min-w-0 overflow-hidden">
                                    <h4 className="text-xs font-bold mb-2">
                                        Response Headers
                                    </h4>
                                    <HeadersTable headers={ex.response.headers} />
                                    <h4 className="text-xs font-bold mt-3 mb-2">
                                        Response Body
                                    </h4>
                                    <BodyViewer
                                        bodyB64={ex.response.body_b64}
                                        truncated={ex.response.body_truncated}
                                    />
                                </div>
                            </div>
                        </div>
                    )}
                </div>
            ))}
        </div>
    );
}
