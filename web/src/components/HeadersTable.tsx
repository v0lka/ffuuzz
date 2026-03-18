interface Props {
    headers?: Record<string, string[]>;
}

export function HeadersTable({ headers }: Props) {
    if (!headers || Object.keys(headers).length === 0) {
        return <span className="text-xs opacity-50">(no headers)</span>;
    }

    return (
        <table className="table table-xs">
            <thead>
                <tr>
                    <th>Header</th>
                    <th>Value</th>
                </tr>
            </thead>
            <tbody>
                {Object.entries(headers).map(([name, values]) => (
                    <tr key={name}>
                        <td className="font-mono text-xs">{name}</td>
                        <td className="font-mono text-xs">{values.join(", ")}</td>
                    </tr>
                ))}
            </tbody>
        </table>
    );
}
