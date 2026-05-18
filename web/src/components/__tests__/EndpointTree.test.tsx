import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import { EndpointTree } from "@/components/EndpointTree";
import type { EndpointFilter } from "@/components/EndpointTree";
import type { TreeOrigin } from "@/types/api";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

// ── Mock hooks ──────────────────────────────────────────────────

const mockDeleteMutate = vi.fn();
const mockAddToCampaignMutate = vi.fn();

vi.mock("@/hooks/queries", async () => {
    const actual = await vi.importActual<typeof import("@/hooks/queries")>(
        "@/hooks/queries",
    );
    return {
        ...actual,
        useRecordingsTree: vi.fn(),
        useDeleteRecordingsByPrefix: () => ({ mutate: mockDeleteMutate }),
        useCampaigns: vi.fn(),
        useAddRecordingsToCampaign: () => ({
            mutate: mockAddToCampaignMutate,
        }),
    };
});

// ── Helpers ────────────────────────────────────────────────────

async function setupImportMocks() {
    const mod = await import("@/hooks/queries");
    return {
        useRecordingsTree: vi.mocked(mod.useRecordingsTree),
        useCampaigns: vi.mocked(mod.useCampaigns),
    };
}

/** Return a UseQueryResult mock. Cast `as any` because strict TanStack Query types require many internal fields. */
function queryResult<T>(data: T) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return {
        data,
        isLoading: false,
        isError: false,
        error: null,
        isPending: false,
        isSuccess: true,
        dataUpdatedAt: 0,
        errorUpdatedAt: 0,
        failureCount: 0,
        failureReason: null,
        isFetched: true,
        isFetchedAfterMount: true,
        isFetching: false,
        isRefetching: false,
        isLoadingError: false,
        isPaused: false,
        isPlaceholderData: false,
        isRefetchError: false,
        isStale: false,
        status: "success" as const,
        refetch: vi.fn(),
        remove: vi.fn(),
        fetchStatus: "idle" as const,
        promise: Promise.resolve(data),
    } as any;
}

// ── Tree data fixtures ─────────────────────────────────────────

const originA: TreeOrigin = {
    origin: "example.com:443",
    scheme: "https",
    host: "example.com",
    port: 443,
    recording_count: 5,
    paths: [],
};

const originB: TreeOrigin = {
    origin: "test.com:80",
    scheme: "http",
    host: "test.com",
    port: 80,
    recording_count: 3,
    paths: [],
};

const originWithPaths: TreeOrigin = {
    origin: "example.com:443",
    scheme: "https",
    host: "example.com",
    port: 443,
    recording_count: 5,
    paths: [
        {
            segment: "api",
            full_path: "/api",
            recording_count: 3,
            children: [
                {
                    segment: "v1",
                    full_path: "/api/v1",
                    recording_count: 1,
                    children: [],
                },
            ],
        },
    ],
};

// ── Tests ──────────────────────────────────────────────────────

describe("EndpointTree", () => {
    let onFilterChange: (filter: EndpointFilter | null) => void;

    beforeEach(async () => {
        vi.clearAllMocks();
        onFilterChange = vi.fn() as any;
        const { useRecordingsTree, useCampaigns } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(queryResult([]));
        useCampaigns.mockReturnValue(queryResult([]));
    });

    // ── 1. Empty array ─────────────────────────────────────────

    it("shows 'No endpoints' when tree is an empty array", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(queryResult([]));

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        expect(screen.getByText("No endpoints")).toBeInTheDocument();
    });

    // ── 2. Undefined tree ──────────────────────────────────────

    it("shows 'No endpoints' when tree is undefined", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(queryResult(undefined));

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        expect(screen.getByText("No endpoints")).toBeInTheDocument();
    });

    // ── 3. Multiple origins ────────────────────────────────────

    it("renders origin nodes with globe icon and recording count badge", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originA, originB]),
        );

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        // Origin labels
        expect(screen.getByText("example.com:443")).toBeInTheDocument();
        expect(screen.getByText("test.com:80")).toBeInTheDocument();

        // Recording count badges are in the same row as the origin label
        const rowA = screen.getByText("example.com:443").closest("div")!;
        const rowB = screen.getByText("test.com:80").closest("div")!;

        expect(within(rowA).getByText("5")).toBeInTheDocument();
        expect(within(rowB).getByText("3")).toBeInTheDocument();

        // Globe icons (lucide svg)
        const svgsInRowA = rowA.querySelectorAll("svg");
        const svgsInRowB = rowB.querySelectorAll("svg");
        expect(svgsInRowA.length).toBeGreaterThan(0);
        expect(svgsInRowB.length).toBeGreaterThan(0);
    });

    // ── 4. Left-click filter ───────────────────────────────────

    it("clicking origin calls onFilterChange with correct EndpointFilter", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originA]),
        );

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        const originEl = screen.getByText("example.com:443");
        await userEvent.click(originEl);

        expect(onFilterChange).toHaveBeenCalledWith<[EndpointFilter]>({
            scheme: "https",
            host: "example.com",
            port: 443,
            pathPrefix: "",
        });
    });

    // ── 5. Toggle off same active origin ───────────────────────

    it("clicking the same active origin again calls onFilterChange(null)", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originA]),
        );

        const activeFilter: EndpointFilter = {
            scheme: "https",
            host: "example.com",
            port: 443,
            pathPrefix: "",
        };

        render(
            <EndpointTree
                onFilterChange={onFilterChange}
                activeFilter={activeFilter}
            />,
            ROUTE,
        );

        const originEl = screen.getByText("example.com:443");
        await userEvent.click(originEl);

        expect(onFilterChange).toHaveBeenCalledWith(null);
    });

    // ── 6. Expand / collapse ───────────────────────────────────

    it("expanding origin shows chevron change and reveals path children", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originWithPaths]),
        );

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        // Path children are not visible initially
        expect(screen.queryByText("api")).not.toBeInTheDocument();

        // Click the chevron (role="button" span inside the origin row)
        const expandButton = screen.getByRole("button");
        await userEvent.click(expandButton);

        // Path children now visible
        expect(screen.getByText("api")).toBeInTheDocument();
    });

    // ── 7. LocalStorage persistence ────────────────────────────

    it("persists expanded state to localStorage across re-mounts", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originWithPaths]),
        );

        const { unmount } = render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        // Expand
        const expandButton = screen.getByRole("button");
        await userEvent.click(expandButton);
        expect(screen.getByText("api")).toBeInTheDocument();

        // Verify localStorage was written
        expect(localStorage.getItem("ffuuzz:tree-expanded")).toContain(
            "example.com:443",
        );

        // Unmount and re-mount a fresh instance -- should read from localStorage
        unmount();

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        // Node should still be expanded
        expect(screen.getByText("api")).toBeInTheDocument();
    });

    // ── 8. Right-click context menu ────────────────────────────

    it("right-click shows context menu with 'Add to campaign' and 'Delete'", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originA]),
        );

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        const originEl = screen.getByText("example.com:443");
        fireEvent.contextMenu(originEl, { clientX: 100, clientY: 200 });

        // Context menu should be visible with both items
        expect(
            screen.getByText("Add to campaign"),
        ).toBeInTheDocument();
        expect(screen.getByText("Delete")).toBeInTheDocument();
    });

    // ── 9. Delete opens ConfirmDialog ──────────────────────────

    it("clicking 'Delete' in context menu opens ConfirmDialog", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originA]),
        );

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        // Open context menu
        const originEl = screen.getByText("example.com:443");
        fireEvent.contextMenu(originEl, { clientX: 100, clientY: 200 });

        // Click "Delete"
        const deleteButton = screen.getByText("Delete");
        await userEvent.click(deleteButton);

        // ConfirmDialog appears
        expect(screen.getByText("Delete Recordings")).toBeInTheDocument();
        expect(
            screen.getByText(
                /Delete all recordings for "example.com:443"/,
            ),
        ).toBeInTheDocument();
    });

    // ── 10. Confirm delete triggers mutation and filter reset ──

    it("ConfirmDialog confirm triggers deleteMutation.mutate and calls onFilterChange(null)", async () => {
        const { useRecordingsTree } = await setupImportMocks();
        useRecordingsTree.mockReturnValue(
            queryResult([originA]),
        );

        render(
            <EndpointTree onFilterChange={onFilterChange} activeFilter={null} />,
            ROUTE,
        );

        // Open context menu
        const originEl = screen.getByText("example.com:443");
        fireEvent.contextMenu(originEl, { clientX: 100, clientY: 200 });

        // Click "Delete" to open ConfirmDialog
        await userEvent.click(screen.getByText("Delete"));

        // Click confirm "Delete" in the dialog
        // The ConfirmDialog's confirm button has class "btn btn-error" and
        // text "Delete" (confirmLabel). Use within to scope to the dialog.
        // Note: <dialog> without the `open` attribute does not have an
        // implicit ARIA "dialog" role in jsdom; use a querySelector instead.
        const dialog = document.querySelector("dialog")!;
        const confirmButton = within(dialog).getByText("Delete");
        await userEvent.click(confirmButton);

        expect(mockDeleteMutate).toHaveBeenCalledWith({
            scheme: "https",
            host: "example.com",
            port: 443,
            path_prefix: undefined,
        });
        expect(onFilterChange).toHaveBeenCalledWith(null);
    });
});
