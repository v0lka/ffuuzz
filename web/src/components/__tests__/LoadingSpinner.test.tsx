import { describe, it, expect } from "vitest";
import { render } from "@/test/test-utils";
import { LoadingSpinner } from "@/components/LoadingSpinner";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("LoadingSpinner", () => {
    it("renders the spinner container", () => {
        const { container } = render(<LoadingSpinner />, ROUTE);
        // The outer div with flex layout
        const outerDiv = container.firstChild as HTMLElement;
        expect(outerDiv).toBeInTheDocument();
        expect(outerDiv.className).toContain("flex");
        expect(outerDiv.className).toContain("items-center");
        expect(outerDiv.className).toContain("justify-center");
    });

    it("the spinner span has the correct DaisyUI classes", () => {
        const { container } = render(<LoadingSpinner />, ROUTE);
        const spinner = container.querySelector("span.loading") as HTMLElement;
        expect(spinner).toBeInTheDocument();
        expect(spinner.className).toContain("loading");
        expect(spinner.className).toContain("loading-spinner");
        expect(spinner.className).toContain("loading-lg");
    });
});
