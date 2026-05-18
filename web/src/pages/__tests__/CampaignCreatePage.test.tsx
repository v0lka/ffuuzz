import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, within } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import CampaignCreatePage from "@/pages/CampaignCreatePage";
import type { RecordingSession, CreateCampaignRequest } from "@/types/api";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

// ── Mock navigate ──────────────────────────────────────────────

const mockNavigate = vi.fn();

vi.mock("react-router-dom", async () => {
    const actual = await vi.importActual<typeof import("react-router-dom")>(
        "react-router-dom",
    );
    return {
        ...actual,
        useNavigate: () => mockNavigate,
    };
});

// ── Mock hooks ─────────────────────────────────────────────────

const mockCreateMutate = vi.fn();

vi.mock("@/hooks/queries", async () => {
    const actual = await vi.importActual<typeof import("@/hooks/queries")>(
        "@/hooks/queries",
    );
    return {
        ...actual,
        useRecordings: vi.fn(),
        useCreateCampaign: vi.fn(() => ({
            mutate: mockCreateMutate,
            isPending: false,
            isError: false,
            error: null,
            isSuccess: false,
            isIdle: true,
            reset: vi.fn(),
        })),
    };
});

// ── Helpers ────────────────────────────────────────────────────

async function setupImportMocks() {
    const mod = await import("@/hooks/queries");
    return {
        useRecordings: vi.mocked(mod.useRecordings),
    };
}

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

// ── Recording fixtures ─────────────────────────────────────────

const mockRecording1: RecordingSession = {
    id: "rec-aaaa-1111",
    schema_version: 1,
    created_at: "2025-06-01T10:00:00Z",
    target: { scheme: "https", host: "example.com", port: 443, path: "/api" },
    entry_count: 10,
};

const mockRecording2: RecordingSession = {
    id: "rec-bbbb-2222",
    schema_version: 1,
    created_at: "2025-06-02T11:00:00Z",
    target: { scheme: "https", host: "example.com", port: 443, path: "/users" },
    entry_count: 5,
};

// ── Tests ──────────────────────────────────────────────────────

describe("CampaignCreatePage", () => {
    beforeEach(async () => {
        vi.clearAllMocks();
        mockCreateMutate.mockImplementation((_req, options) => {
            if (options?.onSuccess) {
                // Simulate successful creation returning campaign with id
                options.onSuccess({ id: "camp-abc-123" });
            }
        });
        // Default: provide two recordings
        const { useRecordings } = await setupImportMocks();
        useRecordings.mockReturnValue(
            queryResult([mockRecording1, mockRecording2]),
        );
    });

    // ── 1. Default values ──────────────────────────────────────

    it("pre-populates form with default values (workers=8, rps=50)", () => {
        render(<CampaignCreatePage />, ROUTE);

        const workersInput = screen.getByDisplayValue("8");
        const rpsInput = screen.getByDisplayValue("50");

        expect(workersInput).toBeInTheDocument();
        expect(rpsInput).toBeInTheDocument();
        // Confirm the inputs have correct labels nearby
        expect(screen.getByText("Workers")).toBeInTheDocument();
        expect(screen.getByText("RPS")).toBeInTheDocument();
    });

    // ── 2. Submit disabled when name empty ─────────────────────

    it("disables submit button when name is empty", () => {
        render(<CampaignCreatePage />, ROUTE);

        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        expect(submitButton).toBeDisabled();
    });

    // ── 3. Submit disabled when no recordings selected ─────────

    it("disables submit button when name is filled but no recordings selected", async () => {
        render(<CampaignCreatePage />, ROUTE);

        const nameInput = screen.getAllByRole("textbox")[0]!;
        await userEvent.type(nameInput, "My Campaign");

        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        expect(submitButton).toBeDisabled();
    });

    // ── 4. Name + recording enables submit ─────────────────────

    it("enables submit when name is filled and a recording is selected", async () => {
        render(<CampaignCreatePage />, ROUTE);

        const nameInput = screen.getAllByRole("textbox")[0]!;
        await userEvent.type(nameInput, "My Campaign");

        // Select the first recording checkbox
        const checkboxes = screen.getAllByRole("checkbox");
        const recordingCheckbox = checkboxes[0]!;
        await userEvent.click(recordingCheckbox);

        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        expect(submitButton).not.toBeDisabled();
    });

    // ── 5. Checkbox toggle adds/removes recording ──────────────

    it("toggling a recording checkbox adds and removes from recording_ids", async () => {
        render(<CampaignCreatePage />, ROUTE);

        const checkboxes = screen.getAllByRole("checkbox");
        const recordingCheckbox = checkboxes[0]!;

        // Initially unchecked
        expect(recordingCheckbox).not.toBeChecked();

        // Check it
        await userEvent.click(recordingCheckbox);
        expect(recordingCheckbox).toBeChecked();

        // Uncheck it
        await userEvent.click(recordingCheckbox);
        expect(recordingCheckbox).not.toBeChecked();
    });

    // ── 6. Submit calls createCampaign.mutate ──────────────────

    it("submitting calls createCampaign.mutate with correct payload", async () => {
        render(<CampaignCreatePage />, ROUTE);

        // Fill name
        const nameInput = screen.getAllByRole("textbox")[0]!;
        await userEvent.type(nameInput, "My Campaign");

        // Select both recordings
        const checkboxes = screen.getAllByRole("checkbox");
        await userEvent.click(checkboxes[0]!);
        await userEvent.click(checkboxes[1]!);

        // Submit
        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        await userEvent.click(submitButton);

        expect(mockCreateMutate).toHaveBeenCalledOnce();

        const [payload] = mockCreateMutate.mock.calls[0] as [
            CreateCampaignRequest,
        ];
        expect(payload.name).toBe("My Campaign");
        expect(payload.recording_ids).toEqual([
            "rec-aaaa-1111",
            "rec-bbbb-2222",
        ]);
        expect(payload.config.limits.workers).toBe(8);
        expect(payload.config.limits.rps).toBe(50);
    });

    // ── 7. Regex patterns from textarea ────────────────────────

    it('converts "pattern1\\npattern2" in regex textarea to anomaly.regex_patterns array', async () => {
        render(<CampaignCreatePage />, ROUTE);

        // Fill name
        const nameInput = screen.getAllByRole("textbox")[0]!;
        await userEvent.type(nameInput, "My Campaign");

        // Select a recording
        const checkboxes = screen.getAllByRole("checkbox");
        await userEvent.click(checkboxes[0]!);

        // Fill regex textarea — find by label text
        const regexLabel = screen.getByText("Regex Patterns (one per line)");
        const card = regexLabel.closest(".card-body") as HTMLElement;
        const textarea = within(card).getByRole("textbox") as HTMLTextAreaElement;
        await userEvent.type(textarea as HTMLElement, "pattern1\npattern2");

        // Submit
        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        await userEvent.click(submitButton);

        const [payload] = mockCreateMutate.mock.calls[0] as [
            CreateCampaignRequest,
        ];
        expect(payload.config.anomaly.regex_patterns).toEqual([
            "pattern1",
            "pattern2",
        ]);
    });

    // ── 8. Empty lines filtered from regex textarea ────────────

    it("filters empty lines from regex textarea when building payload", async () => {
        render(<CampaignCreatePage />, ROUTE);

        // Fill name
        const nameInput = screen.getAllByRole("textbox")[0]!;
        await userEvent.type(nameInput, "My Campaign");

        // Select a recording
        const checkboxes = screen.getAllByRole("checkbox");
        await userEvent.click(checkboxes[0]!);

        // Fill regex textarea with empty lines
        const regexLabel = screen.getByText("Regex Patterns (one per line)");
        const card = regexLabel.closest(".card-body") as HTMLElement;
        const textarea = within(card).getByRole("textbox") as HTMLTextAreaElement;
        await userEvent.type(textarea as HTMLElement, "pattern1\n\npattern2\n\n");

        // Submit
        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        await userEvent.click(submitButton);

        const [payload] = mockCreateMutate.mock.calls[0] as [
            CreateCampaignRequest,
        ];
        expect(payload.config.anomaly.regex_patterns).toEqual([
            "pattern1",
            "pattern2",
        ]);
    });

    // ── 9. On success navigates ────────────────────────────────

    it("navigates to /campaigns/{id} on successful creation", async () => {
        render(<CampaignCreatePage />, ROUTE);

        // Fill name
        const nameInput = screen.getAllByRole("textbox")[0]!;
        await userEvent.type(nameInput, "My Campaign");

        // Select a recording
        const checkboxes = screen.getAllByRole("checkbox");
        await userEvent.click(checkboxes[0]!);

        // Submit
        const submitButton = screen.getByRole("button", {
            name: "Create Campaign",
        });
        await userEvent.click(submitButton);

        expect(mockNavigate).toHaveBeenCalledWith("/campaigns/camp-abc-123");
    });

    // ── 10. Error alert on mutation failure ────────────────────

    it("shows error alert when mutation fails", async () => {
        // Re-mock useCreateCampaign to return isError = true
        const queriesMod = await import("@/hooks/queries");
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        vi.mocked(queriesMod.useCreateCampaign).mockReturnValue({
            mutate: mockCreateMutate,
            isPending: false,
            isError: true,
            error: { name: "Error", message: "Something went wrong" },
            isSuccess: false,
            isIdle: false,
            reset: vi.fn(),
        } as any);

        render(<CampaignCreatePage />, ROUTE);

        expect(screen.getByText("Something went wrong")).toBeInTheDocument();
        expect(screen.getByRole("alert")).toBeInTheDocument();
    });

    // ── 11. No recordings message ──────────────────────────────

    it("shows 'No recordings available' when the recordings list is empty", async () => {
        const { useRecordings } = await setupImportMocks();
        useRecordings.mockReturnValue(queryResult([]));

        render(<CampaignCreatePage />, ROUTE);

        expect(
            screen.getByText("No recordings available"),
        ).toBeInTheDocument();
    });
});
