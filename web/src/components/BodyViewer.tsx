interface Props {
    bodyB64?: string;
    truncated?: boolean;
}

export function BodyViewer({ bodyB64, truncated }: Props) {
    if (!bodyB64) {
        return <span className="text-xs opacity-50">(empty body)</span>;
    }

    let decoded: string;
    try {
        decoded = atob(bodyB64);
    } catch {
        return <span className="text-xs opacity-50">(binary data)</span>;
    }

    // Try to pretty-print JSON
    let display: string;
    try {
        display = JSON.stringify(JSON.parse(decoded), null, 2);
    } catch {
        display = decoded;
    }

    return (
        <div>
            <pre className="bg-base-300 rounded-box p-3 text-xs overflow-x-auto max-h-64 overflow-y-auto">
                <code>{display}</code>
            </pre>
            {truncated && (
                <p className="text-xs text-warning mt-1">Body was truncated</p>
            )}
        </div>
    );
}
