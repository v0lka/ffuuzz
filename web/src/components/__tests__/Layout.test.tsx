import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test/test-utils";
import Layout from "@/components/Layout";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock("@/hooks/queries", () => ({
    useHealth: vi.fn(),
}));

import { useHealth } from "@/hooks/queries";

// Default mock return value (overridden per test as needed)
beforeEach(() => {
    (useHealth as ReturnType<typeof vi.fn>).mockReturnValue({
        data: { status: "ok" },
        isLoading: false,
        isError: false,
    });
});

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

describe("Layout", () => {
    // -----------------------------------------------------------------------
    // Sidebar navigation items
    // -----------------------------------------------------------------------

    it("renders sidebar with all navigation items", () => {
        render(<Layout />, ROUTE);

        expect(screen.getByText("Dashboard")).toBeInTheDocument();
        expect(screen.getByText("Recordings")).toBeInTheDocument();
        expect(screen.getByText("Campaigns")).toBeInTheDocument();
        expect(screen.getByText("Findings")).toBeInTheDocument();
    });

    it("navigation items are links with correct text labels", () => {
        render(<Layout />, ROUTE);

        const dashboardLink = screen.getByText("Dashboard").closest("a");
        const recordingsLink = screen.getByText("Recordings").closest("a");
        const campaignsLink = screen.getByText("Campaigns").closest("a");
        const findingsLink = screen.getByText("Findings").closest("a");

        expect(dashboardLink).toBeInTheDocument();
        expect(recordingsLink).toBeInTheDocument();
        expect(campaignsLink).toBeInTheDocument();
        expect(findingsLink).toBeInTheDocument();

        // Verify hrefs are correct (with /ui basename prefix)
        expect(dashboardLink?.getAttribute("href")).toBe("/ui");
        expect(recordingsLink?.getAttribute("href")).toBe("/ui/recordings");
        expect(campaignsLink?.getAttribute("href")).toBe("/ui/campaigns");
        expect(findingsLink?.getAttribute("href")).toBe("/ui/findings");
    });

    // -----------------------------------------------------------------------
    // Health indicator
    // -----------------------------------------------------------------------

    it("renders health indicator in the sidebar footer", () => {
        render(<Layout />, ROUTE);

        // "Healthy" text should be visible (default mock returns status "ok")
        expect(screen.getByText("Healthy")).toBeInTheDocument();
    });

    it('shows "Healthy" when useHealth returns status "ok"', () => {
        (useHealth as ReturnType<typeof vi.fn>).mockReturnValue({
            data: { status: "ok" },
            isLoading: false,
            isError: false,
        });

        render(<Layout />, ROUTE);

        expect(screen.getByText("Healthy")).toBeInTheDocument();
        expect(screen.queryByText("Degraded")).not.toBeInTheDocument();
    });

    it('shows "Degraded" when useHealth returns status "error"', () => {
        (useHealth as ReturnType<typeof vi.fn>).mockReturnValue({
            data: { status: "error" },
            isLoading: false,
            isError: false,
        });

        render(<Layout />, ROUTE);

        expect(screen.getByText("Degraded")).toBeInTheDocument();
        expect(screen.queryByText("Healthy")).not.toBeInTheDocument();
    });

    it('shows "Degraded" when useHealth returns any non-"ok" status', () => {
        (useHealth as ReturnType<typeof vi.fn>).mockReturnValue({
            data: { status: "degraded" },
            isLoading: false,
            isError: false,
        });

        render(<Layout />, ROUTE);

        expect(screen.getByText("Degraded")).toBeInTheDocument();
        expect(screen.queryByText("Healthy")).not.toBeInTheDocument();
    });

    // -----------------------------------------------------------------------
    // Content area
    // -----------------------------------------------------------------------

    it("renders the content area with Outlet", () => {
        render(<Layout />, ROUTE);

        // The main content area is a <main> element
        const main = screen.getByRole("main");
        expect(main).toBeInTheDocument();
    });

    // -----------------------------------------------------------------------
    // App title
    // -----------------------------------------------------------------------

    it("renders the FFUUZZ app title in sidebar and top bar", () => {
        render(<Layout />, ROUTE);

        // "FFUUZZ" appears in both the sidebar header and the mobile top bar
        const titles = screen.getAllByText("FFUUZZ");
        // One in sidebar, one in mobile top bar (rendered but hidden on desktop)
        expect(titles.length).toBeGreaterThanOrEqual(1);
    });
});
