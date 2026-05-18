import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { EmptyState } from "@/components/EmptyState";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("EmptyState", () => {
    it("renders title", () => {
        render(<EmptyState title="No items found" />, ROUTE);
        expect(screen.getByText("No items found")).toBeInTheDocument();
    });

    it("renders description when provided", () => {
        render(
            <EmptyState
                title="No items found"
                description="Try adjusting your filters"
            />,
            ROUTE,
        );
        expect(
            screen.getByText("Try adjusting your filters"),
        ).toBeInTheDocument();
    });

    it("does NOT render description when not provided", () => {
        render(<EmptyState title="No items found" />, ROUTE);
        expect(
            screen.queryByText("Try adjusting your filters"),
        ).not.toBeInTheDocument();
    });

    it("renders icon when provided", () => {
        render(
            <EmptyState
                title="No items found"
                icon={<span>icon</span>}
            />,
            ROUTE,
        );
        expect(screen.getByText("icon")).toBeInTheDocument();
    });

    it("renders action when provided", () => {
        render(
            <EmptyState
                title="No items found"
                action={<button>click</button>}
            />,
            ROUTE,
        );
        expect(
            screen.getByRole("button", { name: "click" }),
        ).toBeInTheDocument();
    });
});
