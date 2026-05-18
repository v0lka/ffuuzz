import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { HeadersTable } from "@/components/HeadersTable";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("HeadersTable", () => {
    it('shows "(no headers)" when headers is undefined', () => {
        render(<HeadersTable />, ROUTE);
        expect(screen.getByText("(no headers)")).toBeInTheDocument();
    });

    it('shows "(no headers)" when headers is an empty object', () => {
        render(<HeadersTable headers={{}} />, ROUTE);
        expect(screen.getByText("(no headers)")).toBeInTheDocument();
    });

    it("renders table with header/value columns", () => {
        render(
            <HeadersTable
                headers={{ "Content-Type": ["application/json"] }}
            />,
            ROUTE,
        );
        expect(
            screen.getByRole("columnheader", { name: "Header" }),
        ).toBeInTheDocument();
        expect(
            screen.getByRole("columnheader", { name: "Value" }),
        ).toBeInTheDocument();
    });

    it("renders header names and joined values", () => {
        render(
            <HeadersTable
                headers={{ "Content-Type": ["application/json"] }}
            />,
            ROUTE,
        );
        expect(screen.getByText("Content-Type")).toBeInTheDocument();
        expect(screen.getByText("application/json")).toBeInTheDocument();
    });

    it("renders multiple headers correctly", () => {
        render(
            <HeadersTable
                headers={{
                    "Content-Type": ["application/json"],
                    Authorization: ["Bearer token"],
                }}
            />,
            ROUTE,
        );
        expect(screen.getByText("Content-Type")).toBeInTheDocument();
        expect(screen.getByText("application/json")).toBeInTheDocument();
        expect(screen.getByText("Authorization")).toBeInTheDocument();
        expect(screen.getByText("Bearer token")).toBeInTheDocument();
    });
});
