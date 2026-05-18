import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@/test/test-utils";
import { ExchangeViewer } from "@/components/ExchangeViewer";
import type { Exchange } from "@/types/api";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function createExchange(overrides?: Partial<Exchange>): Exchange {
    return {
        request_id: "req-1",
        started_at: "2025-01-01T00:00:00Z",
        duration_ms: 42,
        request: {
            method: "GET",
            path: "/api/test",
            headers: { "Content-Type": ["application/json"] },
            body_b64: btoa('{"key":"value"}'),
            body_truncated: false,
        },
        response: {
            status: 200,
            headers: { "X-Request-Id": ["abc123"] },
            body_b64: btoa('{"result":"ok"}'),
            body_truncated: false,
        },
        ...overrides,
    };
}

// ---------------------------------------------------------------------------
// ExchangeViewer
// ---------------------------------------------------------------------------

describe("ExchangeViewer", () => {
    // -----------------------------------------------------------------------
    // Empty / no entries
    // -----------------------------------------------------------------------

    it("shows placeholder when entries is undefined", () => {
        render(<ExchangeViewer entries={undefined as unknown as Exchange[]} />, ROUTE);
        expect(screen.getByText("No entries available.")).toBeInTheDocument();
    });

    it("shows placeholder when entries is an empty array", () => {
        render(<ExchangeViewer entries={[]} />, ROUTE);
        expect(screen.getByText("No entries available.")).toBeInTheDocument();
    });

    // -----------------------------------------------------------------------
    // Single entry rendering
    // -----------------------------------------------------------------------

    it("renders entry with request method, path, status, and duration", () => {
        const entry = createExchange({
            request: {
                method: "POST",
                path: "/api/users",
                headers: undefined,
                body_b64: undefined,
                body_truncated: false,
            },
            response: {
                status: 200,
                headers: undefined,
                body_b64: undefined,
                body_truncated: false,
            },
            duration_ms: 125,
        });
        render(<ExchangeViewer entries={[entry]} />, ROUTE);

        // MethodBadge renders the method as text inside a span
        expect(screen.getByText("POST")).toBeInTheDocument();
        // Path
        expect(screen.getByText("/api/users")).toBeInTheDocument();
        // Status badge
        expect(screen.getByText("200")).toBeInTheDocument();
        // Duration
        expect(screen.getByText("125ms")).toBeInTheDocument();
    });

    it("shows entry index number (#1, #2, ...)", () => {
        const entries = [
            createExchange({ request_id: "req-a" }),
            createExchange({ request_id: "req-b" }),
        ];
        render(<ExchangeViewer entries={entries} />, ROUTE);

        expect(screen.getByText("#1")).toBeInTheDocument();
        expect(screen.getByText("#2")).toBeInTheDocument();
    });

    // -----------------------------------------------------------------------
    // Expand / collapse
    // -----------------------------------------------------------------------

    it("clicking checkbox toggles expanded content", () => {
        const entry = createExchange();
        render(<ExchangeViewer entries={[entry]} />, ROUTE);

        // Expanded content should NOT be visible initially
        expect(screen.queryByText("Request Headers")).not.toBeInTheDocument();
        expect(screen.queryByText("Response Headers")).not.toBeInTheDocument();
        expect(screen.queryByText("Request Body")).not.toBeInTheDocument();
        expect(screen.queryByText("Response Body")).not.toBeInTheDocument();

        // Click the checkbox to expand
        const checkbox = screen.getByRole("checkbox");
        fireEvent.click(checkbox);

        // Now the expanded content should be visible
        expect(screen.getByText("Request Headers")).toBeInTheDocument();
        expect(screen.getByText("Response Headers")).toBeInTheDocument();
        expect(screen.getByText("Request Body")).toBeInTheDocument();
        expect(screen.getByText("Response Body")).toBeInTheDocument();
    });

    it("only expanded entries show HeadersTable and BodyViewer content", () => {
        const entries = [
            createExchange({ request_id: "req-a" }),
            createExchange({ request_id: "req-b" }),
        ];
        render(<ExchangeViewer entries={entries} />, ROUTE);

        // Neither entry is expanded, so no expanded content
        expect(screen.queryByText("Request Headers")).not.toBeInTheDocument();

        // The header names should be visible from the collapsed HeadersTable though...
        // Actually, when collapsed, the collapse-content div is not rendered at all
        // (due to the {expanded.has(idx) && (...)} conditional).
        // So HeadersTable/BodyViewer are not in the DOM at all.
        // Verify no "Header" or "Value" table headers appear (from HeadersTable).
        expect(screen.queryByRole("columnheader")).not.toBeInTheDocument();
    });

    it("multiple entries each have their own toggle", () => {
        const entries = [
            createExchange({ request_id: "req-a", request: { method: "GET", path: "/a", headers: undefined, body_b64: undefined, body_truncated: false }, response: { status: 200, headers: undefined, body_b64: undefined, body_truncated: false } }),
            createExchange({ request_id: "req-b", request: { method: "POST", path: "/b", headers: undefined, body_b64: undefined, body_truncated: false }, response: { status: 201, headers: undefined, body_b64: undefined, body_truncated: false } }),
        ];
        render(<ExchangeViewer entries={entries} />, ROUTE);

        const checkboxes = screen.getAllByRole("checkbox");
        expect(checkboxes).toHaveLength(2);

        // Expand only the second entry
        fireEvent.click(checkboxes[1]!);

        // Second entry should show expanded content
        expect(screen.getByText("Request Headers")).toBeInTheDocument();
        expect(screen.getByText("Response Headers")).toBeInTheDocument();

        // Now expand the first entry too
        fireEvent.click(checkboxes[0]!);

        // Both are expanded — still works
        // Actually Request Headers appears twice now. Let's verify that
        // expanding one expands only that one by collapsing the second first.
        // We'll just verify the second entry's content stays visible.
        expect(screen.getAllByText("Request Headers")).toHaveLength(2);
    });

    // -----------------------------------------------------------------------
    // Status badge color classes
    // -----------------------------------------------------------------------

    it("status >= 400 gets badge-error class", () => {
        const entry = createExchange({
            response: {
                status: 500,
                headers: undefined,
                body_b64: undefined,
                body_truncated: false,
            },
        });
        render(<ExchangeViewer entries={[entry]} />, ROUTE);

        const badge = screen.getByText("500");
        expect(badge.className).toContain("badge-error");
        expect(badge.className).not.toContain("badge-warning");
        expect(badge.className).not.toContain("badge-success");
    });

    it("status >= 300 and < 400 gets badge-warning class", () => {
        const entry = createExchange({
            response: {
                status: 302,
                headers: undefined,
                body_b64: undefined,
                body_truncated: false,
            },
        });
        render(<ExchangeViewer entries={[entry]} />, ROUTE);

        const badge = screen.getByText("302");
        expect(badge.className).toContain("badge-warning");
        expect(badge.className).not.toContain("badge-error");
        expect(badge.className).not.toContain("badge-success");
    });

    it("status < 300 gets badge-success class", () => {
        const entry = createExchange({
            response: {
                status: 200,
                headers: undefined,
                body_b64: undefined,
                body_truncated: false,
            },
        });
        render(<ExchangeViewer entries={[entry]} />, ROUTE);

        const badge = screen.getByText("200");
        expect(badge.className).toContain("badge-success");
        expect(badge.className).not.toContain("badge-error");
        expect(badge.className).not.toContain("badge-warning");
    });
});
