import { describe, it, expect, vi, beforeEach } from "vitest";
import { get, post, del, delWithParams, parseApiErrorResponse, buildQueryString, ApiClientError } from "@/api/client";

// Mock global fetch
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

beforeEach(() => {
    mockFetch.mockReset();
});

// ── buildQueryString ──────────────────────────────────────────

describe("buildQueryString", () => {
    it("returns empty string for undefined params", () => {
        expect(buildQueryString()).toBe("");
    });

    it("returns empty string for empty params", () => {
        expect(buildQueryString({})).toBe("");
    });

    it("builds query string from params", () => {
        expect(buildQueryString({ limit: 10, offset: 5 })).toBe("?limit=10&offset=5");
    });

    it("excludes undefined values", () => {
        expect(buildQueryString({ limit: 10, host: undefined })).toBe("?limit=10");
    });

    it("excludes empty string values", () => {
        expect(buildQueryString({ host: "" })).toBe("");
    });

    it("converts numbers to strings", () => {
        expect(buildQueryString({ port: 443 })).toBe("?port=443");
    });
});

// ── parseApiErrorResponse ─────────────────────────────────────

describe("parseApiErrorResponse", () => {
    it("parses JSON error body", async () => {
        const res = {
            json: () => Promise.resolve({ error: "NOT_FOUND", message: "Not found", request_id: "abc" }),
        } as Response;

        const result = await parseApiErrorResponse(res);
        expect(result).toEqual({ error: "NOT_FOUND", message: "Not found", request_id: "abc" });
    });

    it("returns fallback on JSON parse failure with default error", async () => {
        const res = {
            json: () => Promise.reject(new Error("parse error")),
            statusText: "Internal Server Error",
        } as Response;

        const result = await parseApiErrorResponse(res);
        expect(result).toEqual({ error: "UNKNOWN", message: "Internal Server Error", request_id: "" });
    });

    it("uses custom defaultError on parse failure", async () => {
        const res = {
            json: () => Promise.reject(new Error("parse error")),
            statusText: "Internal Server Error",
        } as Response;

        const result = await parseApiErrorResponse(res, "CUSTOM_ERROR");
        expect(result).toEqual({ error: "CUSTOM_ERROR", message: "Internal Server Error", request_id: "" });
    });
});

// ── get ────────────────────────────────────────────────────────

describe("get", () => {
    it("returns parsed JSON on 200", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            json: () => Promise.resolve([{ id: "1" }]),
        });

        const result = await get("/api/v1/test", { limit: 10 });
        expect(result).toEqual([{ id: "1" }]);
        expect(mockFetch).toHaveBeenCalledWith("/api/v1/test?limit=10", expect.any(Object));
    });

    it("returns undefined on 204", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 204,
            ok: true,
        });

        const result = await get("/api/v1/test");
        expect(result).toBeUndefined();
    });

    it("throws ApiClientError on non-ok response", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 500,
            ok: false,
            json: () => Promise.resolve({ error: "SERVER_ERROR", message: "Oops", request_id: "req-1" }),
        });

        const err = (await get("/api/v1/test").catch((e) => e)) as ApiClientError;
        expect(err).toBeInstanceOf(ApiClientError);
        expect(err.status).toBe(500);
        expect(err.code).toBe("SERVER_ERROR");
        expect(err.requestId).toBe("req-1");
        expect(err.message).toBe("Oops");
    });

    it("throws with fallback error on non-JSON error body", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 503,
            ok: false,
            statusText: "Service Unavailable",
            json: () => Promise.reject(new Error("not JSON")),
        });

        await expect(get("/api/v1/test")).rejects.toMatchObject({
            status: 503,
            code: "UNKNOWN",
        });
    });

    it("attaches no query string when params are empty", async () => {
        mockFetch.mockResolvedValueOnce({ status: 200, ok: true, json: () => Promise.resolve([]) });
        await get("/api/v1/test", {});
        expect(mockFetch).toHaveBeenCalledWith("/api/v1/test", expect.any(Object));
    });
});

// ── post ───────────────────────────────────────────────────────

describe("post", () => {
    it("sends JSON body with Content-Type header", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 201,
            ok: true,
            json: () => Promise.resolve({ id: "new" }),
        });

        const result = await post("/api/v1/test", { name: "test" });
        expect(result).toEqual({ id: "new" });

        const call = mockFetch.mock.calls[0]!;
        expect(call[1].method).toBe("POST");
        expect(call[1].headers["Content-Type"]).toBe("application/json");
        expect(call[1].body).toBe(JSON.stringify({ name: "test" }));
    });

    it("does not set Content-Type when body is undefined", async () => {
        mockFetch.mockResolvedValueOnce({ status: 200, ok: true, json: () => Promise.resolve({}) });

        await post("/api/v1/test");

        const call = mockFetch.mock.calls[0]!;
        expect(call[1].headers["Content-Type"]).toBeUndefined();
    });
});

// ── del ────────────────────────────────────────────────────────

describe("del", () => {
    it("sends DELETE request and returns void", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            json: () => Promise.resolve(null),
        });

        const result = await del("/api/v1/test/1");
        expect(result).toBeNull();
        expect(mockFetch).toHaveBeenCalledWith("/api/v1/test/1", expect.any(Object));
    });
});

// ── delWithParams ──────────────────────────────────────────────

describe("delWithParams", () => {
    it("attaches query params to DELETE", async () => {
        mockFetch.mockResolvedValueOnce({
            status: 200,
            ok: true,
            json: () => Promise.resolve({ deleted: 3 }),
        });

        const result = await delWithParams("/api/v1/test", { scheme: "https", host: "example.com", port: 443 });
        expect(result).toEqual({ deleted: 3 });
        expect(mockFetch).toHaveBeenCalledWith(
            "/api/v1/test?scheme=https&host=example.com&port=443",
            expect.any(Object),
        );
    });
});

// ── ApiClientError ─────────────────────────────────────────────

describe("ApiClientError", () => {
    it("is an instance of Error", () => {
        const err = new ApiClientError(500, { error: "TEST", message: "Test error", request_id: "r1" });
        expect(err).toBeInstanceOf(Error);
        expect(err.name).toBe("ApiClientError");
        expect(err.message).toBe("Test error");
    });
});
