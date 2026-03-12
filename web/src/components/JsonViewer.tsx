import { useState } from "react";

interface Props {
    data: unknown;
    defaultExpanded?: boolean;
}

export function JsonViewer({ data, defaultExpanded = true }: Props) {
    const [expanded, setExpanded] = useState(defaultExpanded);
    const json = typeof data === "string" ? data : JSON.stringify(data, null, 2);

    return (
        <div className="relative">
            <button
                className="btn btn-ghost btn-xs absolute top-2 right-2 z-10"
                onClick={() => setExpanded(!expanded)}
            >
                {expanded ? "Collapse" : "Expand"}
            </button>
            <pre className="bg-base-300 rounded-box p-4 text-xs overflow-x-auto max-h-96 overflow-y-auto">
                <code>{expanded ? json : json.split("\n").slice(0, 5).join("\n") + "\n..."}</code>
            </pre>
        </div>
    );
}
