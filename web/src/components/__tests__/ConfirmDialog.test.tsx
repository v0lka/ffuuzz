import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@/test/test-utils";
import { ConfirmDialog } from "@/components/ConfirmDialog";

const ROUTE = { initialEntries: ["/ui"] };

// ---------------------------------------------------------------------------
// ConfirmDialog
// ---------------------------------------------------------------------------

describe("ConfirmDialog", () => {
    // ── 1. Title & message ─────────────────────────────────────

    it("renders title and message", () => {
        render(
            <ConfirmDialog
                title="Delete Item"
                message='Are you sure you want to delete "example"?'
                onConfirm={vi.fn()}
                onCancel={vi.fn()}
            />,
            ROUTE,
        );

        expect(screen.getByText("Delete Item")).toBeInTheDocument();
        expect(
            screen.getByText('Are you sure you want to delete "example"?'),
        ).toBeInTheDocument();
    });

    // ── 2. Default confirm label ───────────────────────────────

    it('renders default "Confirm" button label when confirmLabel not provided', () => {
        render(
            <ConfirmDialog
                title="Confirm Action"
                message="Proceed?"
                onConfirm={vi.fn()}
                onCancel={vi.fn()}
            />,
            ROUTE,
        );

        expect(screen.getByText("Confirm")).toBeInTheDocument();
    });

    // ── 3. Custom confirm label ────────────────────────────────

    it("renders custom confirm button label when provided", () => {
        render(
            <ConfirmDialog
                title="Remove Item"
                message="Really?"
                confirmLabel="Delete"
                onConfirm={vi.fn()}
                onCancel={vi.fn()}
            />,
            ROUTE,
        );

        expect(screen.getByText("Delete")).toBeInTheDocument();
    });

    // ── 4. Cancel calls onCancel ───────────────────────────────

    it("clicking Cancel calls onCancel", async () => {
        const onCancel = vi.fn();
        render(
            <ConfirmDialog
                title="T"
                message="M"
                onConfirm={vi.fn()}
                onCancel={onCancel}
            />,
            ROUTE,
        );

        const cancelBtn = screen.getByText("Cancel");
        cancelBtn.click();
        expect(onCancel).toHaveBeenCalledTimes(1);
    });

    // ── 5. Confirm calls onConfirm ─────────────────────────────

    it("clicking Confirm calls onConfirm", async () => {
        const onConfirm = vi.fn();
        render(
            <ConfirmDialog
                title="T"
                message="M"
                onConfirm={onConfirm}
                onCancel={vi.fn()}
            />,
            ROUTE,
        );

        const confirmBtn = screen.getByText("Confirm");
        confirmBtn.click();
        expect(onConfirm).toHaveBeenCalledTimes(1);
    });

    // ── 6. Backdrop calls onCancel ─────────────────────────────

    it("clicking backdrop calls onCancel", () => {
        const onCancel = vi.fn();
        render(
            <ConfirmDialog
                title="T"
                message="M"
                onConfirm={vi.fn()}
                onCancel={onCancel}
            />,
            ROUTE,
        );

        const backdrop = document.querySelector(".modal-backdrop")!;
        fireEvent.click(backdrop);
        expect(onCancel).toHaveBeenCalledTimes(1);
    });
});
