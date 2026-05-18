import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@/test/test-utils";
import { JsonViewer } from "@/components/JsonViewer";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("JsonViewer", () => {
    const testObj = { name: "Alice", age: 30, city: "Wonderland" };

    it("renders full JSON when defaultExpanded is true (default)", () => {
        render(<JsonViewer data={testObj} />, ROUTE);
        // Should show all keys from the object
        expect(screen.getByText(/"name"/)).toBeInTheDocument();
        expect(screen.getByText(/"Alice"/)).toBeInTheDocument();
        expect(screen.getByText(/"age"/)).toBeInTheDocument();
        expect(screen.getByText(/"city"/)).toBeInTheDocument();
        // Should NOT show the "..." truncation suffix
        expect(screen.queryByText(/\.\.\./)).not.toBeInTheDocument();
    });

    it("shows Collapse button when expanded", () => {
        render(<JsonViewer data={testObj} />, ROUTE);
        expect(screen.getByRole("button", { name: "Collapse" })).toBeInTheDocument();
    });

    it("shows collapsed view when defaultExpanded is false", () => {
        render(<JsonViewer data={testObj} defaultExpanded={false} />, ROUTE);
        // Should show "..." at the end
        expect(screen.getByText(/\.\.\./)).toBeInTheDocument();
        // Should show the Expand button
        expect(screen.getByRole("button", { name: "Expand" })).toBeInTheDocument();
    });

    it("shows Expand button when collapsed", () => {
        render(<JsonViewer data={testObj} defaultExpanded={false} />, ROUTE);
        expect(screen.getByRole("button", { name: "Expand" })).toBeInTheDocument();
        expect(screen.queryByRole("button", { name: "Collapse" })).not.toBeInTheDocument();
    });

    it("clicking Expand button shows full JSON and button changes to Collapse", () => {
        render(<JsonViewer data={testObj} defaultExpanded={false} />, ROUTE);

        // Initially collapsed
        const expandBtn = screen.getByRole("button", { name: "Expand" });
        expect(expandBtn).toBeInTheDocument();

        // Click Expand
        fireEvent.click(expandBtn);

        // Now should show full JSON and Collapse button
        expect(screen.getByRole("button", { name: "Collapse" })).toBeInTheDocument();
        expect(screen.getByText(/"name"/)).toBeInTheDocument();
        expect(screen.getByText(/"Alice"/)).toBeInTheDocument();
        expect(screen.getByText(/"age"/)).toBeInTheDocument();
        expect(screen.getByText(/"city"/)).toBeInTheDocument();
    });

    it("works with string data (passed as-is) vs object data (JSON.stringify-d)", () => {
        // String data: rendered as-is
        const { unmount } = render(<JsonViewer data='{"x":1}' />, ROUTE);
        expect(screen.getByText('{"x":1}')).toBeInTheDocument();
        unmount();

        // Object data: JSON.stringify-d
        render(<JsonViewer data={{ x: 1 }} />, ROUTE);
        // Pretty-printed: should have newlines/indentation, so "x" with quotes
        expect(screen.getByText(/"x"/)).toBeInTheDocument();
        expect(screen.getByText(/1/)).toBeInTheDocument();
    });
});
