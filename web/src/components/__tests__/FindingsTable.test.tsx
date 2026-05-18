import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { FindingsTable } from "@/components/FindingsTable";
import type { Finding } from "@/types/api";

const ROUTE = { initialEntries: ["/ui"] };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function createTestFinding(overrides: Partial<Finding> = {}): Finding {
    return {
        id: "f-001",
        campaign_id: "camp-abc12345",
        type: "TIMEOUT",
        status: "UNCONFIRMED",
        signature: "sig-1",
        method: "GET",
        endpoint: "/api/v1/users",
        details: {},
        minimized: false,
        mutation_type: "json:string_mutation",
        mutation_payload: '{"key":"value"}',
        created_at: "2025-01-01T00:00:00Z",
        ...overrides,
    };
}

// ---------------------------------------------------------------------------
// FindingsTable
// ---------------------------------------------------------------------------

describe("FindingsTable", () => {
    // ── 1. Column headers ──────────────────────────────────────

    it("renders column headers: Type, Status, Method, Endpoint, Mutation, Payload, When", () => {
        render(<FindingsTable findings={[createTestFinding()]} />, ROUTE);

        expect(screen.getByText("Type")).toBeInTheDocument();
        expect(screen.getByText("Status")).toBeInTheDocument();
        expect(screen.getByText("Method")).toBeInTheDocument();
        expect(screen.getByText("Endpoint")).toBeInTheDocument();
        expect(screen.getByText("Mutation")).toBeInTheDocument();
        expect(screen.getByText("Payload")).toBeInTheDocument();
        expect(screen.getByText("When")).toBeInTheDocument();
    });

    // ── 2. Campaign column when showCampaign=true ──────────────

    it("renders Campaign column when showCampaign=true", () => {
        render(
            <FindingsTable
                findings={[createTestFinding()]}
                showCampaign={true}
                campaignNames={new Map([["camp-abc12345", "My Campaign"]])}
            />,
            ROUTE,
        );

        expect(screen.getByText("Campaign")).toBeInTheDocument();
    });

    // ── 3. No Campaign column when showCampaign=false ──────────

    it("does NOT render Campaign column when showCampaign=false", () => {
        render(
            <FindingsTable
                findings={[createTestFinding()]}
                showCampaign={false}
            />,
            ROUTE,
        );

        expect(screen.queryByText("Campaign")).not.toBeInTheDocument();
    });

    // ── 4. Finding type badge with correct color ───────────────

    it("renders finding type badges with correct colors", () => {
        render(
            <FindingsTable
                findings={[createTestFinding({ type: "TIMEOUT" })]}
            />,
            ROUTE,
        );

        const badge = screen.getByText("TIMEOUT");
        expect(badge.className).toContain("badge-warning");
        expect(badge.className).toContain("badge");
    });

    // ── 5. Mutation type truncated ─────────────────────────────

    it("renders mutation type truncated (after last colon)", () => {
        render(
            <FindingsTable
                findings={[createTestFinding({ mutation_type: "json:string_mutation" })]}
            />,
            ROUTE,
        );

        // formatMutationType strips everything before (and including) the last colon,
        // producing "string_mutation". Since it is 15 chars (fits in 18), no middle-truncation.
        expect(screen.getByText("string_mutation")).toBeInTheDocument();
    });

    // ── 6. Mutation payload via TruncatedEndpoint ──────────────

    it("renders mutation_payload using TruncatedEndpoint", () => {
        render(
            <FindingsTable
                findings={[createTestFinding({ mutation_payload: '{"foo":"bar"}' })]}
            />,
            ROUTE,
        );

        // TruncatedEndpoint renders the value as text with a title tooltip.
        const el = screen.getByText('{"foo":"bar"}');
        expect(el).toBeInTheDocument();
        expect(el).toHaveAttribute("title", '{"foo":"bar"}');
    });

    // ── 7. Campaign name from campaignNames Map ────────────────

    it("links campaign name when showCampaign=true with campaignNames Map", () => {
        render(
            <FindingsTable
                findings={[createTestFinding({ campaign_id: "camp-abc12345" })]}
                showCampaign={true}
                campaignNames={new Map([["camp-abc12345", "My Campaign"]])}
            />,
            ROUTE,
        );

        // The link text is the name from the Map, not the ID.
        const link = screen.getByText("My Campaign");
        expect(link).toBeInTheDocument();
        expect(link).toHaveAttribute("href", "/ui/campaigns/camp-abc12345");
    });

    // ── 8. Fallback to truncated campaign_id ───────────────────

    it("falls back to campaign_id truncated when no campaignNames provided", () => {
        render(
            <FindingsTable
                findings={[createTestFinding({ campaign_id: "camp-longid12345" })]}
                showCampaign={true}
            />,
            ROUTE,
        );

        // Without a campaignNames map, the component shows the first 8 chars of the id.
        const link = screen.getByText("camp-lon");
        expect(link).toBeInTheDocument();
        expect(link).toHaveAttribute("href", "/ui/campaigns/camp-longid12345");
    });
});
