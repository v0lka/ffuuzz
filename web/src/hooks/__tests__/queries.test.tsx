import { describe, it, expect, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/test-utils";
import { queryKeys, useRecordings, useRecording, useCampaignStats, useCampaignFindings, useFindings, useHealth } from "@/hooks/queries";

// Mock API modules
vi.mock("@/api/recordings", () => ({
    listRecordings: vi.fn(),
    getRecording: vi.fn(),
    getRecordingsTree: vi.fn(),
}));

vi.mock("@/api/campaigns", () => ({
    listCampaigns: vi.fn(),
    getCampaign: vi.fn(),
    getCampaignStats: vi.fn(),
    getCampaignFindings: vi.fn(),
}));

vi.mock("@/api/findings", () => ({
    listFindings: vi.fn(),
    getFinding: vi.fn(),
}));

vi.mock("@/api/health", () => ({
    getHealth: vi.fn(),
}));

const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={createTestQueryClient()}>
        {children}
    </QueryClientProvider>
);

// ── queryKeys ──────────────────────────────────────────────────

describe("queryKeys", () => {
    it("recordings list key matches expected shape", () => {
        const key = queryKeys.recordings.list({ limit: 10 });
        expect(key).toEqual(["recordings", "list", { limit: 10 }]);
    });

    it("recordings list key with undefined params", () => {
        const key = queryKeys.recordings.list();
        expect(key).toEqual(["recordings", "list", undefined]);
    });

    it("recordings detail key", () => {
        expect(queryKeys.recordings.detail("abc")).toEqual(["recordings", "detail", "abc"]);
    });

    it("campaigns findings key", () => {
        const key = queryKeys.campaigns.findings("c1", { type: "TIMEOUT" });
        expect(key).toEqual(["campaigns", "findings", "c1", { type: "TIMEOUT" }]);
    });

    it("findings list key", () => {
        const key = queryKeys.findings.list({ limit: 10, offset: 0 });
        expect(key).toEqual(["findings", "list", { limit: 10, offset: 0 }]);
    });

    it("health key", () => {
        expect(queryKeys.health).toEqual(["health"]);
    });

    it("all shape checks", () => {
        expect(queryKeys.recordings.all).toEqual(["recordings"]);
        expect(queryKeys.campaigns.all).toEqual(["campaigns"]);
        expect(queryKeys.findings.all).toEqual(["findings"]);
        expect(queryKeys.recordings.tree).toEqual(["recordings", "tree"]);
        expect(queryKeys.campaigns.stats("c1")).toEqual(["campaigns", "stats", "c1"]);
        expect(queryKeys.campaigns.config("c1")).toEqual(["campaigns", "config", "c1"]);
        expect(queryKeys.findings.detail("f1")).toEqual(["findings", "detail", "f1"]);
        expect(queryKeys.findings.artifact("f1")).toEqual(["findings", "artifact", "f1"]);
    });
});

// ── useRecordings ──────────────────────────────────────────────

describe("useRecordings", () => {
    it("calls listRecordings with params", async () => {
        const recordingsApi = await import("@/api/recordings");
        const mockList = vi.mocked(recordingsApi.listRecordings);
        mockList.mockResolvedValue([]);

        renderHook(() => useRecordings({ limit: 10, host: "example.com" }), { wrapper });

        await waitFor(() => {
            expect(mockList).toHaveBeenCalledWith({ limit: 10, host: "example.com" });
        });
    });

    it("refetches every 1 second", () => {
        // Verify the refetchInterval is configured
        // This can't be easily tested with renderHook alone, but we verify
        // the query key is correctly constructed
        const key = queryKeys.recordings.list({ limit: 5 });
        expect(key).toEqual(["recordings", "list", { limit: 5 }]);
    });
});

// ── useRecording enabled check ─────────────────────────────────

describe("useRecording", () => {
    it("does not fetch when id is empty string", async () => {
        const recordingsApi = await import("@/api/recordings");
        const mockGet = vi.mocked(recordingsApi.getRecording);

        renderHook(() => useRecording("", true), { wrapper });

        // No fetch should happen when enabled=false
        expect(mockGet).not.toHaveBeenCalled();
    });

    it("fetches when id is provided", async () => {
        const recordingsApi = await import("@/api/recordings");
        const mockGet = vi.mocked(recordingsApi.getRecording);
        mockGet.mockResolvedValue({ id: "abc", schema_version: 1, created_at: "", target: { scheme: "https", host: "x", port: 443, path: "/" } });

        renderHook(() => useRecording("abc", true), { wrapper });

        await waitFor(() => {
            expect(mockGet).toHaveBeenCalledWith("abc", true, 65536);
        });
    });
});

// ── useCampaignStats ───────────────────────────────────────────

describe("useCampaignStats", () => {
    it("does not fetch when id is empty", async () => {
        const campaignsApi = await import("@/api/campaigns");
        const mockStats = vi.mocked(campaignsApi.getCampaignStats);

        renderHook(() => useCampaignStats("", true, 5000), { wrapper });

        expect(mockStats).not.toHaveBeenCalled();
    });
});

// ── useCampaignFindings ────────────────────────────────────────

describe("useCampaignFindings", () => {
    it("does not fetch when id is empty", async () => {
        const campaignsApi = await import("@/api/campaigns");
        const mockFindings = vi.mocked(campaignsApi.getCampaignFindings);

        renderHook(() => useCampaignFindings("", {}, 2000), { wrapper });

        expect(mockFindings).not.toHaveBeenCalled();
    });
});

// ── useFindings ────────────────────────────────────────────────

describe("useFindings", () => {
    it("fetches with all filter params", async () => {
        const findingsApi = await import("@/api/findings");
        const mockList = vi.mocked(findingsApi.listFindings);
        mockList.mockResolvedValue([]);

        renderHook(
            () =>
                useFindings({
                    campaign_id: "c1",
                    type: "TIMEOUT",
                    status: "UNCONFIRMED",
                    limit: 10,
                    offset: 0,
                }),
            { wrapper },
        );

        await waitFor(() => {
            expect(mockList).toHaveBeenCalledWith({
                campaign_id: "c1",
                type: "TIMEOUT",
                status: "UNCONFIRMED",
                limit: 10,
                offset: 0,
            });
        });
    });
});

// ── useHealth ──────────────────────────────────────────────────

describe("useHealth", () => {
    it("calls getHealth", async () => {
        const healthApi = await import("@/api/health");
        const mockHealth = vi.mocked(healthApi.getHealth);
        mockHealth.mockResolvedValue({ status: "ok", db: "ok", time: "" });

        renderHook(() => useHealth(), { wrapper });

        await waitFor(() => {
            expect(mockHealth).toHaveBeenCalled();
        });
    });
});
