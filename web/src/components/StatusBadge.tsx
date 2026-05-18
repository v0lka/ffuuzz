import type { CampaignStatus, FindingType, FindingStatus } from "@/types/api";

/**
 * Renders an endpoint string with CSS text-overflow: ellipsis when it doesn't fit.
 * The full value is available in the title tooltip on hover.
 */
export function TruncatedEndpoint({ value }: { value: string }) {
    return (
        <span
            className="font-mono text-xs block overflow-hidden whitespace-nowrap text-ellipsis"
            title={value}
        >
            {value}
        </span>
    );
}

const campaignStatusColors: Record<CampaignStatus, string> = {
    CREATED: "badge-neutral",
    STARTING: "badge-info",
    RUNNING: "badge-success",
    STOPPING: "badge-warning",
    STOPPED: "badge-ghost",
    FINISHED: "badge-primary",
    FAILED: "badge-error",
};

export const findingTypeColors: Record<FindingType, string> = {
    TIMEOUT: "badge-warning",
    SERVER_ERROR: "badge-error",
    LATENCY_REGRESSION: "badge-info",
    REGEX_MATCH: "badge-secondary",
};

const findingStatusColors: Record<FindingStatus, string> = {
    UNCONFIRMED: "badge-ghost",
    CONFIRMED: "badge-success",
};

export function CampaignStatusBadge({ status }: { status: CampaignStatus }) {
    return (
        <span className={`badge badge-sm ${campaignStatusColors[status]}`}>
            {status}
        </span>
    );
}

export function FindingTypeBadge({ type, httpStatus }: { type: FindingType; httpStatus?: number }) {
    const label =
        type === "SERVER_ERROR" && httpStatus != null
            ? String(httpStatus)
            : type.replace(/_/g, " ");
    return (
        <span className={`badge badge-sm ${findingTypeColors[type]}`}>
            {label}
        </span>
    );
}

export function FindingStatusBadge({ status }: { status: FindingStatus }) {
    return (
        <span className={`badge badge-sm ${findingStatusColors[status]}`}>
            {status}
        </span>
    );
}

const methodColors: Record<string, string> = {
    GET: "badge-info",
    POST: "badge-success",
    PUT: "badge-warning",
    PATCH: "badge-warning",
    DELETE: "badge-error",
};

export function MethodBadge({ method }: { method: string }) {
    return (
        <span className={`badge badge-sm ${methodColors[method] ?? "badge-ghost"}`}>
            {method}
        </span>
    );
}
