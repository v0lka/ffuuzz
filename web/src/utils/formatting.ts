import type { FindingType } from "@/types/api";

/**
 * Truncates a string in the middle with ellipsis if it exceeds maxLen.
 */
export function truncateMiddle(str: string, maxLen: number): string {
    if (str.length <= maxLen) return str;
    const half = Math.floor((maxLen - 1) / 2);
    return str.slice(0, half) + "\u2026" + str.slice(-half);
}

/**
 * Truncates a mutation type string for display.
 * Shows only the last part after the colon (e.g., "json:string_mutation" -> "string_mutation").
 */
export function formatMutationType(mutationType: string | undefined): string {
    if (!mutationType) return "-";
    const parts = mutationType.split(":");
    if (parts.length > 1) {
        return parts[parts.length - 1] ?? mutationType;
    }
    return mutationType;
}

/**
 * Maps FindingType to a short single-word display label.
 */
export const findingTypeLabels: Record<FindingType, string> = {
    TIMEOUT: "TIMEOUT",
    SERVER_ERROR: "5xx",
    LATENCY_REGRESSION: "LATENCY",
    REGEX_MATCH: "REGEX",
};
