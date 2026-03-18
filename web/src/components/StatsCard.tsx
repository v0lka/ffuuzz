import type { ReactNode } from "react";

interface Props {
    icon?: ReactNode;
    label: string;
    value: string | number;
    className?: string;
}

export function StatsCard({ icon, label, value, className = "" }: Props) {
    return (
        <div className={`stat bg-base-200 rounded-box ${className}`}>
            {icon && <div className="stat-figure text-primary">{icon}</div>}
            <div className="stat-title text-xs">{label}</div>
            <div className="stat-value text-2xl">{value}</div>
        </div>
    );
}
