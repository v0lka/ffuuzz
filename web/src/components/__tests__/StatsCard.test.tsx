import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { StatsCard } from "@/components/StatsCard";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("StatsCard", () => {
    it("renders label and value", () => {
        render(<StatsCard label="Total Requests" value={42} />, ROUTE);
        expect(screen.getByText("Total Requests")).toBeInTheDocument();
        expect(screen.getByText("42")).toBeInTheDocument();
    });

    it("renders icon when provided", () => {
        render(
            <StatsCard
                label="Total Requests"
                value={42}
                icon={<span>icon</span>}
            />,
            ROUTE,
        );
        expect(screen.getByText("icon")).toBeInTheDocument();
    });

    it("does NOT render icon when not provided", () => {
        render(<StatsCard label="Total Requests" value={42} />, ROUTE);
        expect(screen.queryByText("icon")).not.toBeInTheDocument();
    });

    it("has default stat classes", () => {
        render(<StatsCard label="Total Requests" value={42} />, ROUTE);
        // The outermost div should have stat, bg-base-200, rounded-box.
        // getByText returns the stat-title div; parentElement is the outer div.
        const statDiv = screen.getByText("Total Requests").parentElement;
        expect(statDiv?.className).toContain("stat");
        expect(statDiv?.className).toContain("bg-base-200");
        expect(statDiv?.className).toContain("rounded-box");
    });

    it("appends custom className", () => {
        render(
            <StatsCard
                label="Total Requests"
                value={42}
                className="custom-extra"
            />,
            ROUTE,
        );
        const statDiv = screen.getByText("Total Requests").parentElement;
        expect(statDiv?.className).toContain("custom-extra");
    });
});
