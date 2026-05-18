import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import CampaignDetailPage from "@/pages/CampaignDetailPage";

// ── Route constant (MemoryRouter basename="/ui") ───────────────

const ROUTE = { initialEntries: ["/ui"] };

// ── Mutation fns ──────────────────────────────────────────────

const startMutate = vi.fn();
const stopMutate = vi.fn();

// ── Mock react-router-dom: override useParams ─────────────────

vi.mock("react-router-dom", async () => {
    const actual = await vi.importActual<typeof import("react-router-dom")>(
        "react-router-dom",
    );
    return {
        ...actual,
        useParams: () => ({ id: "c1" }),
    };
});

// ── Mock hooks/queries ────────────────────────────────────────

vi.mock("@/hooks/queries", async () => {
    const actual = await vi.importActual<typeof import("@/hooks/queries")>(
        "@/hooks/queries",
    );
    return {
        ...actual,
        useCampaign: vi.fn(),
        useCampaignConfig: vi.fn(),
        useCampaignStats: vi.fn(),
        useCampaignFindings: vi.fn(),
        useStartCampaign: vi.fn(() => ({
            mutate: startMutate,
            isPending: false,
        })),
        useStopCampaign: vi.fn(() => ({
            mutate: stopMutate,
            isPending: false,
        })),
    };
});

// ── Mock useSSE ───────────────────────────────────────────────

const streamResult = { isConnected: false, error: null };

vi.mock("@/hooks/useSSE", () => ({
    useCampaignStream: vi.fn(() => streamResult),
}));

// ── Helpers ───────────────────────────────────────────────────

/** Minimal query-result-like object returned by react-query hooks */
function qResult<T>(
    data: T,
    overrides?: Partial<{ isLoading: boolean; error: Error | null }>,
) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return { data, isLoading: false, error: null, ...overrides } as any;
}

async function setupMocks() {
    const queriesMod = await import("@/hooks/queries");
    const sseMod = await import("@/hooks/useSSE");
    return {
        useCampaign: vi.mocked(queriesMod.useCampaign),
        useCampaignConfig: vi.mocked(queriesMod.useCampaignConfig),
        useCampaignStats: vi.mocked(queriesMod.useCampaignStats),
        useCampaignFindings: vi.mocked(queriesMod.useCampaignFindings),
        useCampaignStream: vi.mocked(sseMod.useCampaignStream),
    };
}

// ── Fixtures ──────────────────────────────────────────────────

const mockCampaign = {
    id: "c1",
    name: "Test Campaign",
    status: "CREATED" as const,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    recording_ids: ["rec1", "rec2"],
};

const mockRunningCampaign = {
    ...mockCampaign,
    status: "RUNNING" as const,
};

const mockConfig = {
    target: { base_url: "https://example.com" },
    limits: {
        workers: 4,
        rps: 100,
        max_tests: 1000,
        duration_sec: 60,
        req_timeout_ms: 5000,
    },
    mutations: {
        path_query: true,
        headers: false,
        json_body: true,
        params: true,
        sequence: false,
        intensity: 5,
    },
    anomaly: { detect_5xx: true, latency_multiplier: 3 },
    triage: { confirm_runs: 1, enable_minimization: false },
};

// ── Tests ─────────────────────────────────────────────────────

describe("CampaignDetailPage", () => {
    beforeEach(async () => {
        vi.clearAllMocks();
        const mocks = await setupMocks();
        // Default: valid campaign with CREATED status
        mocks.useCampaign.mockReturnValue(qResult(mockCampaign));
        mocks.useCampaignConfig.mockReturnValue(qResult(mockConfig));
        mocks.useCampaignStats.mockReturnValue(qResult(null));
        mocks.useCampaignFindings.mockReturnValue(qResult([]));
        mocks.useCampaignStream.mockReturnValue({
            isConnected: false,
            error: null,
        });
    });

    // ── 1. Loading state ──────────────────────────────────────

    it("shows loading spinner when campaign is loading", async () => {
        const { useCampaign } = await setupMocks();
        useCampaign.mockReturnValue(
            qResult(undefined as never, { isLoading: true }),
        );

        render(<CampaignDetailPage />, ROUTE);

        expect(
            document.querySelector(".loading-spinner"),
        ).toBeInTheDocument();
        expect(screen.queryByText("Test Campaign")).not.toBeInTheDocument();
    });

    // ── 2. Error state ────────────────────────────────────────

    it("shows error when campaign not found", async () => {
        const { useCampaign } = await setupMocks();
        useCampaign.mockReturnValue(
            qResult(undefined as never, {
                error: new Error("not found"),
            }),
        );

        render(<CampaignDetailPage />, ROUTE);

        expect(screen.getByText("Campaign not found")).toBeInTheDocument();
    });

    // ── 3. Campaign name ──────────────────────────────────────

    it("renders campaign name", () => {
        render(<CampaignDetailPage />, ROUTE);
        expect(screen.getByText("Test Campaign")).toBeInTheDocument();
    });

    // ── 4. Status badge ───────────────────────────────────────

    it("renders campaign status badge", () => {
        render(<CampaignDetailPage />, ROUTE);
        expect(screen.getByText("CREATED")).toBeInTheDocument();
    });

    // ── 5. Start button when CREATED ─────────────────────────

    it('shows "Start" button when status is CREATED', () => {
        render(<CampaignDetailPage />, ROUTE);
        expect(
            screen.getByRole("button", { name: /start/i }),
        ).toBeInTheDocument();
    });

    // ── 6. No Start button when RUNNING ──────────────────────

    it('hides "Start" button when status is RUNNING', async () => {
        const { useCampaign } = await setupMocks();
        useCampaign.mockReturnValue(qResult(mockRunningCampaign));

        render(<CampaignDetailPage />, ROUTE);

        expect(
            screen.queryByRole("button", { name: /start/i }),
        ).not.toBeInTheDocument();
    });

    // ── 7. Stop button when RUNNING ──────────────────────────

    it('shows "Stop" button when status is RUNNING', async () => {
        const { useCampaign } = await setupMocks();
        useCampaign.mockReturnValue(qResult(mockRunningCampaign));

        render(<CampaignDetailPage />, ROUTE);

        expect(
            screen.getByRole("button", { name: /stop/i }),
        ).toBeInTheDocument();
    });

    // ── 8. Clicking Start calls startMutation.mutate ──────────

    it("clicking Start calls startMutation.mutate with campaign id", async () => {
        render(<CampaignDetailPage />, ROUTE);

        const startButton = screen.getByRole("button", { name: /start/i });
        await userEvent.click(startButton);

        expect(startMutate).toHaveBeenCalledWith("c1");
    });

    // ── 9. Tab switching – Config tab ────────────────────────

    it("switches to Config tab and renders JsonViewer", async () => {
        render(<CampaignDetailPage />, ROUTE);

        const configTab = screen.getByRole("tab", { name: /config/i });
        await userEvent.click(configTab);

        // JsonViewer renders a Collapse button when expanded by default
        expect(screen.getByText("Collapse")).toBeInTheDocument();
    });

    // ── 10. Info tab shows recording links ───────────────────

    it("shows recording links on Info tab", async () => {
        render(<CampaignDetailPage />, ROUTE);

        const infoTab = screen.getByRole("tab", { name: /info/i });
        await userEvent.click(infoTab);

        expect(
            screen.getByRole("link", { name: "rec1" }),
        ).toBeInTheDocument();
        expect(
            screen.getByRole("link", { name: "rec2" }),
        ).toBeInTheDocument();
    });

    // ── 11. No findings yet ──────────────────────────────────

    it('shows "No findings yet" when findings tab is active and list is empty', () => {
        render(<CampaignDetailPage />, ROUTE);

        // Default tab is "findings", data is empty array
        expect(screen.getByText("No findings yet")).toBeInTheDocument();
    });

    // ── 12. No SSE indicator when not active ─────────────────

    it("does not show SSE indicator when status is not active", () => {
        render(<CampaignDetailPage />, ROUTE);

        expect(screen.queryByText("Live")).not.toBeInTheDocument();
        expect(screen.queryByText("Reconnecting")).not.toBeInTheDocument();
    });

    // ── 13. SSE Live indicator when RUNNING and connected ────

    it('shows SSE "Live" indicator when isConnected and status is RUNNING', async () => {
        const { useCampaign, useCampaignStream } = await setupMocks();
        useCampaign.mockReturnValue(qResult(mockRunningCampaign));
        useCampaignStream.mockReturnValue({
            isConnected: true,
            error: null,
        });

        render(<CampaignDetailPage />, ROUTE);

        expect(screen.getByText("Live")).toBeInTheDocument();
    });
});
