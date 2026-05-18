import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@/test/test-utils";
import { ImportDialog } from "@/components/ImportDialog";
import type { RecordingSession } from "@/types/api";

const ROUTE = { initialEntries: ["/ui"] };

// ── Mock hooks ──────────────────────────────────────────────────

const mockImportMutate = vi.fn();
const mockImportReset = vi.fn();

// Default mutation state; individual tests override via vi.mocked(...).
const defaultMutationState = {
    mutate: mockImportMutate,
    isPending: false,
    isError: false,
    isSuccess: false,
    data: undefined as unknown,
    error: null as Error | null,
    reset: mockImportReset,
};

vi.mock("@/hooks/queries", async () => {
    const actual = await vi.importActual<typeof import("@/hooks/queries")>(
        "@/hooks/queries",
    );
    return {
        ...actual,
        useImportRecordings: vi.fn(),
    };
});

// ── Helpers ────────────────────────────────────────────────────

/**
 * Creates a minimal valid RecordingSession object for file upload tests.
 */
function makeSession(overrides: Partial<RecordingSession> = {}): RecordingSession {
    return {
        schema_version: 1,
        id: "s-001",
        created_at: "2025-01-01T00:00:00Z",
        target: { scheme: "https", host: "example.com", port: 443, path: "/" },
        ...overrides,
    };
}

/**
 * Returns a File object containing the given JSON-serializable data.
 */
function makeJsonFile(data: unknown, name = "test.json"): File {
    return new File([JSON.stringify(data)], name, {
        type: "application/json",
    });
}

/** Triggers a file selection on the hidden file input inside ImportDialog. */
function selectFile(file: File) {
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(input, { target: { files: [file] } });
}

async function setupImportMock() {
    const mod = await import("@/hooks/queries");
    const useImportRecordings = vi.mocked(mod.useImportRecordings);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    useImportRecordings.mockReturnValue(defaultMutationState as any);
    return { useImportRecordings };
}

// ── Tests ──────────────────────────────────────────────────────

describe("ImportDialog", () => {
    beforeEach(async () => {
        vi.clearAllMocks();
        await setupImportMock();
    });

    // ── 1. Returns null when closed ────────────────────────────

    it("returns null when open=false", () => {
        const { container } = render(
            <ImportDialog open={false} onClose={vi.fn()} />,
            ROUTE,
        );
        expect(container.innerHTML).toBe("");
    });

    // ── 2. Renders modal title ─────────────────────────────────

    it("renders modal with title 'Import Recordings' when open=true", () => {
        render(<ImportDialog open={true} onClose={vi.fn()} />, ROUTE);
        expect(screen.getByText("Import Recordings")).toBeInTheDocument();
    });

    // ── 3. Import button disabled when no file loaded ──────────

    it("shows 'Import' button disabled when no file loaded", () => {
        render(<ImportDialog open={true} onClose={vi.fn()} />, ROUTE);
        const importBtn = screen.getByText("Import");
        expect(importBtn).toBeDisabled();
    });

    // ── 4. Cancel calls onClose ────────────────────────────────

    it("clicking Cancel calls onClose", () => {
        const onClose = vi.fn();
        render(<ImportDialog open={true} onClose={onClose} />, ROUTE);
        const cancelBtn = screen.getByText("Cancel");
        cancelBtn.click();
        expect(onClose).toHaveBeenCalledTimes(1);
    });

    // ── 5. Modal backdrop calls onClose ────────────────────────

    it("clicking modal backdrop calls onClose", () => {
        const onClose = vi.fn();
        render(<ImportDialog open={true} onClose={onClose} />, ROUTE);
        const backdrop = document.querySelector(".modal-backdrop")!;
        fireEvent.click(backdrop);
        expect(onClose).toHaveBeenCalledTimes(1);
    });

    // ── 6. Valid JSON with RecordingSession array ──────────────

    it("shows session count and enables Import after valid file selection", async () => {
        render(<ImportDialog open={true} onClose={vi.fn()} />, ROUTE);

        const sessions: RecordingSession[] = [makeSession(), makeSession({ id: "s-002" })];
        selectFile(makeJsonFile(sessions));

        await waitFor(() => {
            expect(
                screen.getByText("2 session(s) ready to import"),
            ).toBeInTheDocument();
        });

        const importBtn = screen.getByText("Import");
        expect(importBtn).toBeEnabled();
    });

    // ── 7. Invalid JSON shows parse error ──────────────────────

    it("shows parse error after selecting invalid JSON", async () => {
        render(<ImportDialog open={true} onClose={vi.fn()} />, ROUTE);

        const file = new File(["not valid json {{{"], "bad.json", {
            type: "application/json",
        });
        selectFile(file);

        await waitFor(() => {
            expect(
                screen.getByText("Failed to parse JSON file"),
            ).toBeInTheDocument();
        });
    });

    // ── 8. Invalid session objects ─────────────────────────────

    it("shows 'Invalid recording session format' for malformed objects", async () => {
        render(<ImportDialog open={true} onClose={vi.fn()} />, ROUTE);

        // Objects missing required "id" and "target" fields
        selectFile(makeJsonFile([{ foo: "bar" }]));

        await waitFor(() => {
            expect(
                screen.getByText("Invalid recording session format"),
            ).toBeInTheDocument();
        });
    });

    // ── 9. Import calls mutate; onSuccess resets & closes ─────

    it("clicking Import calls mutate and onSuccess resets and closes", async () => {
        // Configure mutate to invoke the onSuccess callback synchronously
        mockImportMutate.mockImplementationOnce(
            (_sessions: unknown, options?: { onSuccess?: () => void }) => {
                options?.onSuccess?.();
            },
        );

        const onClose = vi.fn();
        render(<ImportDialog open={true} onClose={onClose} />, ROUTE);

        // Select a valid file
        selectFile(makeJsonFile([makeSession()]));
        await waitFor(() => {
            expect(screen.getByText("1 session(s) ready to import")).toBeInTheDocument();
        });

        // Click Import
        const importBtn = screen.getByText("Import");
        importBtn.click();

        // Verify mutate was called
        expect(mockImportMutate).toHaveBeenCalledTimes(1);
        // The sessions array should be passed as the first argument
        const sessionsArg = mockImportMutate.mock.calls[0]?.[0];
        expect(Array.isArray(sessionsArg)).toBe(true);
        expect(sessionsArg).toHaveLength(1);

        // onSuccess should have triggered onClose
        expect(onClose).toHaveBeenCalledTimes(1);
    });

    // ── 10. Error message from mutation ────────────────────────

    it("shows error message when importMutation.isError is true", async () => {
        const { useImportRecordings } = await import("@/hooks/queries");
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        vi.mocked(useImportRecordings).mockReturnValue({
            ...defaultMutationState,
            isError: true,
            error: new Error("test error"),
        } as any);

        render(<ImportDialog open={true} onClose={vi.fn()} />, ROUTE);

        expect(screen.getByText("test error")).toBeInTheDocument();
    });
});
