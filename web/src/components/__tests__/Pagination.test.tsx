import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import { Pagination } from "@/components/Pagination";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------
describe("Pagination", () => {
    describe("page number", () => {
        it("renders Page 1 when offset=0, limit=20", () => {
            render(
                <Pagination
                    offset={0}
                    limit={20}
                    hasMore={true}
                    onPageChange={() => {}}
                />,
                ROUTE,
            );
            expect(screen.getByText("Page 1")).toBeInTheDocument();
        });

        it("renders Page 3 when offset=40, limit=20", () => {
            render(
                <Pagination
                    offset={40}
                    limit={20}
                    hasMore={true}
                    onPageChange={() => {}}
                />,
                ROUTE,
            );
            expect(screen.getByText("Page 3")).toBeInTheDocument();
        });

        it("renders Page 3 when offset=20, limit=10", () => {
            render(
                <Pagination
                    offset={20}
                    limit={10}
                    hasMore={true}
                    onPageChange={() => {}}
                />,
                ROUTE,
            );
            expect(screen.getByText("Page 3")).toBeInTheDocument();
        });
    });

    describe("Previous button", () => {
        it("is disabled when offset is 0", () => {
            render(
                <Pagination
                    offset={0}
                    limit={20}
                    hasMore={true}
                    onPageChange={() => {}}
                />,
                ROUTE,
            );
            expect(screen.getByText("Previous")).toBeDisabled();
        });

        it("calls onPageChange with offset - limit when clicked", async () => {
            const onPageChange = vi.fn();
            render(
                <Pagination
                    offset={40}
                    limit={20}
                    hasMore={true}
                    onPageChange={onPageChange}
                />,
                ROUTE,
            );

            const user = userEvent.setup();
            await user.click(screen.getByText("Previous"));

            expect(onPageChange).toHaveBeenCalledTimes(1);
            expect(onPageChange).toHaveBeenCalledWith(20);
        });

        it("clamps to 0 when offset < limit (edge case: offset=10, limit=20)", async () => {
            const onPageChange = vi.fn();
            render(
                <Pagination
                    offset={10}
                    limit={20}
                    hasMore={true}
                    onPageChange={onPageChange}
                />,
                ROUTE,
            );

            const user = userEvent.setup();
            await user.click(screen.getByText("Previous"));

            expect(onPageChange).toHaveBeenCalledTimes(1);
            expect(onPageChange).toHaveBeenCalledWith(0);
        });
    });

    describe("Next button", () => {
        it("is disabled when hasMore is false", () => {
            render(
                <Pagination
                    offset={0}
                    limit={20}
                    hasMore={false}
                    onPageChange={() => {}}
                />,
                ROUTE,
            );
            expect(screen.getByText("Next")).toBeDisabled();
        });

        it("is enabled when hasMore is true", () => {
            render(
                <Pagination
                    offset={0}
                    limit={20}
                    hasMore={true}
                    onPageChange={() => {}}
                />,
                ROUTE,
            );
            expect(screen.getByText("Next")).toBeEnabled();
        });

        it("calls onPageChange with offset + limit when clicked", async () => {
            const onPageChange = vi.fn();
            render(
                <Pagination
                    offset={20}
                    limit={20}
                    hasMore={true}
                    onPageChange={onPageChange}
                />,
                ROUTE,
            );

            const user = userEvent.setup();
            await user.click(screen.getByText("Next"));

            expect(onPageChange).toHaveBeenCalledTimes(1);
            expect(onPageChange).toHaveBeenCalledWith(40);
        });
    });
});
