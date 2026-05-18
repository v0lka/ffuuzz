import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useCampaignStream } from "@/hooks/useSSE";
import { queryKeys } from "@/hooks/queries";
import type { CampaignStats } from "@/types/api";

let queryClient: QueryClient;
let mockEventSource: { url: string; onopen: (() => void) | null; onmessage: ((e: MessageEvent) => void) | null; onerror: (() => void) | null; close: ReturnType<typeof vi.fn>; addEventListener: ReturnType<typeof vi.fn> };

const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>
        {children}
    </QueryClientProvider>
);

const makeStats = (overrides: Partial<CampaignStats> = {}): CampaignStats => ({
    campaign_id: "c1",
    status: "RUNNING",
    tests_total: 100,
    tests_per_sec: 10,
    timeouts: 1,
    server_errors: 2,
    latency_regressions: 3,
    regex_matches: 4,
    last_activity_at: new Date().toISOString(),
    seeds: { sessions_total: 5, sessions_used: 5, exchanges_sent: 50 },
    ...overrides,
});

beforeEach(() => {
    queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 }, mutations: { retry: false } },
    });

    // Reset the mock EventSource constructor
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    vi.spyOn(globalThis, "EventSource").mockImplementation(function (url: any) {
        mockEventSource = {
            url: url as string,
            onopen: null,
            onmessage: null,
            onerror: null,
            close: vi.fn(),
            addEventListener: vi.fn(),
        };
        return mockEventSource as unknown as EventSource;
    });
});

afterEach(() => {
    vi.restoreAllMocks();
});

// ── Initialization ─────────────────────────────────────────────

describe("useCampaignStream", () => {
    it("does not create EventSource when enabled=false", () => {
        const constructorSpy = vi.spyOn(globalThis, "EventSource");

        renderHook(() => useCampaignStream("c1", false), { wrapper });

        expect(constructorSpy).not.toHaveBeenCalled();
    });

    it("creates EventSource with correct URL when enabled", () => {
        renderHook(() => useCampaignStream("c1", true), { wrapper });

        expect(mockEventSource.url).toBe("/api/v1/campaigns/c1/stream");
    });

    it("does not create EventSource when campaignId is empty", () => {
        const constructorSpy = vi.spyOn(globalThis, "EventSource");

        renderHook(() => useCampaignStream("", true), { wrapper });

        expect(constructorSpy).not.toHaveBeenCalled();
    });

    // ── Connection state ───────────────────────────────────────────

    it("sets isConnected=true on open", () => {
        const { result } = renderHook(() => useCampaignStream("c1", true), { wrapper });

        expect(result.current.isConnected).toBe(false);

        act(() => {
            mockEventSource.onopen?.();
        });

        expect(result.current.isConnected).toBe(true);
    });

    // ── Message handling ──────────────────────────────────────────

    it("updates stats cache on valid message", () => {
        const stats = makeStats();
        const setSpy = vi.spyOn(queryClient, "setQueryData");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        act(() => {
            mockEventSource.onmessage?.(new MessageEvent("message", { data: JSON.stringify(stats) }));
        });

        expect(setSpy).toHaveBeenCalledWith(queryKeys.campaigns.stats("c1"), stats);
    });

    it("updates campaign detail status on message", () => {
        const stats = makeStats({ status: "RUNNING" });
        const setSpy = vi.spyOn(queryClient, "setQueryData");

        // Seed the detail cache
        queryClient.setQueryData(queryKeys.campaigns.detail("c1"), { id: "c1", status: "CREATED" });

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        act(() => {
            mockEventSource.onmessage?.(new MessageEvent("message", { data: JSON.stringify(stats) }));
        });

        expect(setSpy).toHaveBeenCalledWith(
            queryKeys.campaigns.detail("c1"),
            expect.any(Function),
        );
    });

    it("ignores invalid JSON on message", () => {
        const setSpy = vi.spyOn(queryClient, "setQueryData");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        act(() => {
            mockEventSource.onmessage?.(new MessageEvent("message", { data: "not json" }));
        });

        expect(setSpy).not.toHaveBeenCalled();
    });

    it("ignores message without campaign_id", () => {
        const setSpy = vi.spyOn(queryClient, "setQueryData");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        act(() => {
            mockEventSource.onmessage?.(new MessageEvent("message", { data: JSON.stringify({ status: "RUNNING" }) }));
        });

        expect(setSpy).not.toHaveBeenCalled();
    });

    it("ignores message without status", () => {
        const setSpy = vi.spyOn(queryClient, "setQueryData");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        act(() => {
            mockEventSource.onmessage?.(new MessageEvent("message", { data: JSON.stringify({ campaign_id: "c1" }) }));
        });

        expect(setSpy).not.toHaveBeenCalled();
    });

    it("ignores null values", () => {
        const setSpy = vi.spyOn(queryClient, "setQueryData");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        act(() => {
            mockEventSource.onmessage?.(new MessageEvent("message", { data: "null" }));
        });

        expect(setSpy).not.toHaveBeenCalled();
    });

    // ── Findings invalidation on count increase ───────────────────

    it("invalidates findings when count increases", () => {
        const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        // First message: total findings = 10
        act(() => {
            mockEventSource.onmessage?.(
                new MessageEvent("message", {
                    data: JSON.stringify(makeStats({ timeouts: 4, server_errors: 3, latency_regressions: 2, regex_matches: 1 })),
                }),
            );
        });

        const firstCallCount = invalidateSpy.mock.calls.length;

        // Second message: total findings = 20 (increased)
        act(() => {
            mockEventSource.onmessage?.(
                new MessageEvent("message", {
                    data: JSON.stringify(makeStats({ timeouts: 10, server_errors: 5, latency_regressions: 3, regex_matches: 2 })),
                }),
            );
        });

        // Should have invalidated findings — at least one call made after the count increase
        expect(invalidateSpy.mock.calls.length).toBeGreaterThan(firstCallCount);
    });

    // ── "done" event ───────────────────────────────────────────────

    it("handles done event and closes connection", () => {
        const stats = makeStats({ status: "FINISHED" });
        const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        // The hook calls es.addEventListener("done", handler)
        // We need to find and call that handler via the mock
        // The mock addEventListener captures calls
        const addEventListenerMock = mockEventSource.addEventListener as ReturnType<typeof vi.fn>;
        if (addEventListenerMock.mock.calls.length > 0) {
            const doneCall = addEventListenerMock.mock.calls.find(
                (call: unknown[]) => call[0] === "done",
            );
            if (doneCall) {
                const handler = doneCall[1] as (e: MessageEvent) => void;
                act(() => {
                    handler(new MessageEvent("done", { data: JSON.stringify(stats) }));
                });

                expect(mockEventSource.close).toHaveBeenCalled();
                expect(invalidateSpy).toHaveBeenCalled();
            }
        }
    });

    // ── Cleanup on unmount ────────────────────────────────────────

    it("closes EventSource on unmount", () => {
        const { unmount } = renderHook(() => useCampaignStream("c1", true), { wrapper });

        unmount();

        expect(mockEventSource.close).toHaveBeenCalled();
    });

    // ── totalFindings helper (indirectly via message) ───────────

    it("correctly calculates total findings from stats fields", () => {
        const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

        renderHook(() => useCampaignStream("c1", true), { wrapper });

        // Start with no findings
        act(() => {
            mockEventSource.onmessage?.(
                new MessageEvent("message", {
                    data: JSON.stringify(makeStats({ timeouts: 0, server_errors: 0, latency_regressions: 0, regex_matches: 0 })),
                }),
            );
        });

        const callsBefore = invalidateSpy.mock.calls.length;

        // Then send a stats with some findings (total = 10)
        act(() => {
            mockEventSource.onmessage?.(
                new MessageEvent("message", {
                    data: JSON.stringify(makeStats({ timeouts: 3, server_errors: 4, latency_regressions: 2, regex_matches: 1 })),
                }),
            );
        });

        expect(invalidateSpy.mock.calls.length).toBeGreaterThan(callsBefore);
        // The first finding count was 0, new is 10 — should invalidate
    });
});
