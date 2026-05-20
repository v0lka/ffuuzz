interface Props {
    headers?: Record<string, string[]>;
}

export function HeadersTable({ headers }: Props) {
    if (!headers || Object.keys(headers).length === 0) {
        return <span className="text-xs opacity-50">(no headers)</span>;
    }

    return (
        <div className="overflow-x-auto">
            <table style={{ tableLayout: "fixed", width: "100%" }}>
                <thead>
                    <tr className="border-b border-base-300">
                        <th className="text-left text-xs font-semibold py-1 px-2 w-2/5">Header</th>
                        <th className="text-left text-xs font-semibold py-1 px-2 w-3/5">Value</th>
                    </tr>
                </thead>
                <tbody>
                    {Object.entries(headers).map(([name, values]) => (
                        <tr key={name} className="border-b border-base-300/50">
                            <td className="font-mono text-xs py-1 px-2 align-top" style={{ wordBreak: "break-all", whiteSpace: "normal" }}>{name}</td>
                            <td className="font-mono text-xs py-1 px-2 align-top" style={{ wordBreak: "break-all", whiteSpace: "normal" }}>{values.join(", ")}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
