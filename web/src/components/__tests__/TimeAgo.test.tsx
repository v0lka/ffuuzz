import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import { TimeAgo, formatDateTime } from "@/components/TimeAgo";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

describe("TimeAgo", () => {
    it("renders relative time for a valid ISO date", () => {
        const oneHourAgo = new Date(Date.now() - 3600000).toISOString();
        render(<TimeAgo date={oneHourAgo} />, ROUTE);
        const span = screen.getByText(/about 1 hour ago/i);
        expect(span).toBeInTheDocument();
    });

    it("sets the title attribute to the original date string", () => {
        const date = "2024-01-15T12:00:00Z";
        render(<TimeAgo date={date} />, ROUTE);
        const span = screen.getByTitle(date);
        expect(span).toBeInTheDocument();
        expect(span.tagName).toBe("SPAN");
    });

    it("falls back to raw date string on invalid date input", () => {
        const invalidDate = "not-a-date";
        render(<TimeAgo date={invalidDate} />, ROUTE);
        const span = screen.getByText(invalidDate);
        expect(span).toBeInTheDocument();
        // The span should NOT have a title attribute (the catch branch renders just <span>{date}</span>)
        expect(span).not.toHaveAttribute("title");
    });
});

describe("formatDateTime", () => {
    it("returns a locale string for a valid date", () => {
        const localeString = formatDateTime("2024-06-15T14:30:00Z");
        // Should produce a localized date-time string, not the raw input
        expect(localeString).toBeTruthy();
        expect(localeString).not.toBe("2024-06-15T14:30:00Z");
    });

    it("returns 'Invalid Date' for an unparseable date string", () => {
        // parseISO does not throw on invalid input; it returns an Invalid Date
        // whose toLocaleString() produces "Invalid Date"
        const result = formatDateTime("not-a-date");
        expect(result).toBe("Invalid Date");
    });
});
