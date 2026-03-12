import { useEffect, useRef, useState } from "react";
import type { CampaignStatus, FindingType, FindingStatus } from "@/types/api";

/**
 * Renders an endpoint string truncated in the middle when it doesn't fit,
 * replacing the removed part with the Unicode ellipsis character (...).
 */
export function TruncatedEndpoint({ value }: { value: string }) {
    const containerRef = useRef<HTMLSpanElement>(null);
    const measureRef = useRef<HTMLSpanElement | null>(null);
    const [display, setDisplay] = useState(value);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;

        const compute = () => {
            // Temporarily show full text to measure natural width
            el.textContent = value;
            const available = el.offsetWidth;
            if (available <= 0 || el.scrollWidth <= available) {
                setDisplay(value);
                return;
            }

            // Binary-search the longest prefix+suffix that fits
            // Use a hidden measuring span with the same font
            const measure = document.createElement("span");
            measure.style.cssText =
                "position:absolute;visibility:hidden;white-space:nowrap;" +
                "font:" + getComputedStyle(el).font;
            document.body.appendChild(measure);
            measureRef.current = measure;

            const measureWidth = (s: string) => {
                measure.textContent = s;
                return measure.offsetWidth;
            };

            const ellipsis = "\u2026";
            const half = Math.floor(value.length / 2);
            let lo = 0;
            let hi = half;
            let result = ellipsis;

            while (lo <= hi) {
                const mid = Math.floor((lo + hi) / 2);
                const prefix = value.slice(0, mid);
                const suffix = value.slice(value.length - mid);
                const candidate = prefix + ellipsis + suffix;
                if (measureWidth(candidate) <= available) {
                    result = candidate;
                    lo = mid + 1;
                } else {
                    hi = mid - 1;
                }
            }

            document.body.removeChild(measure);
            measureRef.current = null;
            setDisplay(result);
        };

        compute();
        const ro = new ResizeObserver(compute);
        ro.observe(el);
        return () => {
            ro.disconnect();
            if (measureRef.current?.parentNode) {
                measureRef.current.parentNode.removeChild(measureRef.current);
                measureRef.current = null;
            }
        };
    }, [value]);

    return (
        <span
            ref={containerRef}
            className="font-mono text-xs block w-full overflow-hidden whitespace-nowrap"
            title={value}
        >
            {display}
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
