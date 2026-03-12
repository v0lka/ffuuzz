import { formatDistanceToNow, parseISO } from "date-fns";

export function TimeAgo({ date }: { date: string }) {
    try {
        return (
            <span title={date}>
                {formatDistanceToNow(parseISO(date), { addSuffix: true })}
            </span>
        );
    } catch {
        return <span>{date}</span>;
    }
}

export function formatDateTime(date: string): string {
    try {
        return parseISO(date).toLocaleString();
    } catch {
        return date;
    }
}
