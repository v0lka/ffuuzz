import { describe, it, expect, vi, beforeEach } from "vitest";
import { ApiClientError } from "@/api/client";
import { listRecordings, exportRecordings } from "@/api/recordings";

// Mock global fetch
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

beforeEach(() => {
    mockFetch.mockReset();
});

// ── listRecordings ────────────────────────────────────────────

describe("listRecordings", () => {
    it("returns empty array when response is null", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            json: () => Promise.resolve(null),
        });

        const result = await listRecordings();
        expect(result).toEqual([]);
    });

    it("passes params to fetch URL", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            json: () => Promise.resolve([]),
        });

        await listRecordings({ host: "example.com", path_prefix: "/api/v1" });

        const calls = mockFetch.mock.calls;
        expect(calls).toHaveLength(1);
        const url = calls[0]![0];
        expect(url).toContain("/api/v1/recordings?");
        expect(url).toContain("host=example.com");
        expect(url).toContain("path_prefix=");
    });
});

// ── exportRecordings ──────────────────────────────────────────

describe("exportRecordings", () => {
    it("creates a download link and calls fetch with correct URL", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            blob: () =>
                Promise.resolve(
                    new Blob(["test"], { type: "application/json" }),
                ),
        });

        await exportRecordings();

        expect(mockFetch).toHaveBeenCalledWith("/api/v1/recordings/export");
    });

    it("throws ApiClientError on non-ok response", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 500,
            ok: false,
            statusText: "Internal Server Error",
            json: () =>
                Promise.resolve({
                    error: "EXPORT_FAILED",
                    message: "Export failed",
                    request_id: "req-1",
                }),
        });

        const err = await exportRecordings().catch((e) => e);
        expect(err).toBeInstanceOf(ApiClientError);
        expect(err.status).toBe(500);
        expect(err.code).toBe("EXPORT_FAILED");
        expect(err.message).toBe("Export failed");
    });

    it("handles export with params in URL", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            blob: () =>
                Promise.resolve(
                    new Blob(["test"], { type: "application/json" }),
                ),
        });

        await exportRecordings({
            host: "example.com",
            path_prefix: "/api/v1",
        });

        const calls = mockFetch.mock.calls;
        expect(calls).toHaveLength(1);
        const url = calls[0]![0] as string;
        expect(url).toContain("/api/v1/recordings/export?");
        expect(url).toContain("host=example.com");
    });
});
