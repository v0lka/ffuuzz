import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { ErrorAlert } from "@/components/ErrorAlert";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("ErrorAlert", () => {
    it("renders the error message", () => {
        render(<ErrorAlert message="Something went wrong" />, ROUTE);
        expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    });

    it("has role=\"alert\"", () => {
        render(<ErrorAlert message="Something went wrong" />, ROUTE);
        expect(screen.getByRole("alert")).toBeInTheDocument();
    });

    it("shows Request ID when requestId is provided", () => {
        render(
            <ErrorAlert
                message="Something went wrong"
                requestId="abc-123"
            />,
            ROUTE,
        );
        expect(screen.getByText("Request ID: abc-123")).toBeInTheDocument();
    });

    it("does NOT show Request ID when requestId is undefined", () => {
        render(<ErrorAlert message="Something went wrong" />, ROUTE);
        expect(
            screen.queryByText(/Request ID:/),
        ).not.toBeInTheDocument();
    });
});
