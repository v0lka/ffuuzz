import { useEffect, useRef, useState, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { Campaign, CampaignStats } from "@/types/api";
import { queryKeys } from "./queries";

function isValidStats(parsed: unknown): parsed is CampaignStats {
    return (
        parsed != null &&
        typeof parsed === "object" &&
        "campaign_id" in parsed &&
        "status" in parsed
    );
}

function totalFindings(stats: CampaignStats): number {
    return (
        (stats.timeouts ?? 0) +
        (stats.server_errors ?? 0) +
        (stats.latency_regressions ?? 0) +
        (stats.regex_matches ?? 0)
    );
}

interface UseCampaignStreamResult {
    isConnected: boolean;
    error: Error | null;
}

export function useCampaignStream(
    campaignId: string,
    enabled: boolean,
): UseCampaignStreamResult {
    const queryClient = useQueryClient();
    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);
    const esRef = useRef<EventSource | null>(null);
    const retriesRef = useRef(0);
    const maxRetries = 3;
    const prevFindingCountRef = useRef(-1);

    const connect = useCallback(() => {
        if (!enabled || !campaignId) return;

        const url = `/api/v1/campaigns/${campaignId}/stream`;
        const es = new EventSource(url);
        esRef.current = es;

        es.onopen = () => {
            setIsConnected(true);
            setError(null);
            retriesRef.current = 0;
            prevFindingCountRef.current = -1;
        };

        es.onmessage = (event: MessageEvent) => {
            try {
                const parsed: unknown = JSON.parse(String(event.data));
                if (!isValidStats(parsed)) return;
                const stats = parsed;
                queryClient.setQueryData(
                    queryKeys.campaigns.stats(campaignId),
                    stats,
                );
                // Also update campaign status in the detail cache
                queryClient.setQueryData<Campaign>(
                    queryKeys.campaigns.detail(campaignId),
                    (old) => (old ? { ...old, status: stats.status } : old),
                );
                // Invalidate findings when the count increases
                const count = totalFindings(stats);
                if (count > prevFindingCountRef.current) {
                    prevFindingCountRef.current = count;
                    // Use prefix matching by passing only the first 3 elements
                    void queryClient.invalidateQueries({
                        queryKey: ["campaigns", "findings", campaignId],
                    });
                    // Also invalidate global findings list
                    void queryClient.invalidateQueries({
                        queryKey: ["findings", "list"],
                    });
                }
            } catch {
                // Ignore parse errors
            }
        };

        es.addEventListener("done", (event: MessageEvent) => {
            try {
                const parsed: unknown = JSON.parse(String(event.data));
                if (isValidStats(parsed)) {
                    queryClient.setQueryData(
                        queryKeys.campaigns.stats(campaignId),
                        parsed,
                    );
                }
            } catch {
                // Ignore
            }
            es.close();
            setIsConnected(false);
            // Invalidate to get final state
            void queryClient.invalidateQueries({
                queryKey: ["campaigns", "detail", campaignId],
            });
            void queryClient.invalidateQueries({
                queryKey: ["campaigns", "list"],
            });
            void queryClient.invalidateQueries({
                queryKey: ["campaigns", "findings", campaignId],
            });
            void queryClient.invalidateQueries({
                queryKey: ["findings", "list"],
            });
        });

        es.onerror = () => {
            es.close();
            setIsConnected(false);

            if (retriesRef.current < maxRetries) {
                retriesRef.current++;
                const delay = Math.min(1000 * 2 ** retriesRef.current, 8000);
                setTimeout(connect, delay);
            } else {
                setError(new Error("SSE connection failed after retries"));
            }
        };
    }, [campaignId, enabled, queryClient]);

    useEffect(() => {
        connect();

        return () => {
            if (esRef.current) {
                esRef.current.close();
                esRef.current = null;
            }
            setIsConnected(false);
        };
    }, [connect]);

    return { isConnected, error };
}
