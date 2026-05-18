import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { BodyViewer } from "@/components/BodyViewer";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("BodyViewer", () => {
    it("shows (empty body) when bodyB64 is undefined", () => {
        render(<BodyViewer />, ROUTE);
        expect(screen.getByText("(empty body)")).toBeInTheDocument();
    });

    it("shows (empty body) when bodyB64 is empty string", () => {
        render(<BodyViewer bodyB64="" />, ROUTE);
        expect(screen.getByText("(empty body)")).toBeInTheDocument();
    });

    it("shows (binary data) for invalid base64", () => {
        render(<BodyViewer bodyB64="!!not-valid-base64!!" />, ROUTE);
        expect(screen.getByText("(binary data)")).toBeInTheDocument();
    });

    it("shows pretty-printed JSON for valid JSON base64 input", () => {
        const b64 = btoa('{"hello":"world"}');
        render(<BodyViewer bodyB64={b64} />, ROUTE);
        // The pretty-printed JSON should appear in the document
        expect(screen.getByText(/"hello"/)).toBeInTheDocument();
        expect(screen.getByText(/"world"/)).toBeInTheDocument();
    });

    it("shows Body was truncated warning when truncated=true, and not when false or undefined", () => {
        const b64 = btoa('{"key":"value"}');

        // truncated=true → warning visible
        const { unmount } = render(<BodyViewer bodyB64={b64} truncated={true} />, ROUTE);
        expect(screen.getByText("Body was truncated")).toBeInTheDocument();
        unmount();

        // truncated=false → no warning
        render(<BodyViewer bodyB64={b64} truncated={false} />, ROUTE);
        expect(screen.queryByText("Body was truncated")).not.toBeInTheDocument();
        unmount();

        // truncated=undefined → no warning
        render(<BodyViewer bodyB64={b64} />, ROUTE);
        expect(screen.queryByText("Body was truncated")).not.toBeInTheDocument();
    });
});
