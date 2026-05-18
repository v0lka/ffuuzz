import { describe, it, expect } from "vitest";
import { render, screen } from "@/test/test-utils";
import {
    CampaignStatusBadge,
    FindingTypeBadge,
    FindingStatusBadge,
    MethodBadge,
    TruncatedEndpoint,
} from "@/components/StatusBadge";
import type { CampaignStatus, FindingType, FindingStatus } from "@/types/api";

// The test-utils MemoryRouter uses basename="/ui", so all render calls must
// provide an initial route that starts with the basename.
const ROUTE = { initialEntries: ["/ui"] };

// ---------------------------------------------------------------------------
// CampaignStatusBadge
// ---------------------------------------------------------------------------
describe("CampaignStatusBadge", () => {
    const cases: Array<{
        status: CampaignStatus;
        expectedClass: string;
        expectedLabel: string;
    }> = [
        { status: "CREATED", expectedClass: "badge-neutral", expectedLabel: "CREATED" },
        { status: "STARTING", expectedClass: "badge-info", expectedLabel: "STARTING" },
        { status: "RUNNING", expectedClass: "badge-success", expectedLabel: "RUNNING" },
        { status: "STOPPING", expectedClass: "badge-warning", expectedLabel: "STOPPING" },
        { status: "STOPPED", expectedClass: "badge-ghost", expectedLabel: "STOPPED" },
        { status: "FINISHED", expectedClass: "badge-primary", expectedLabel: "FINISHED" },
        { status: "FAILED", expectedClass: "badge-error", expectedLabel: "FAILED" },
    ];

    it.each(cases)(
        "renders $status with $expectedClass",
        ({ status, expectedClass, expectedLabel }) => {
            render(<CampaignStatusBadge status={status} />, ROUTE);
            const badge = screen.getByText(expectedLabel);
            expect(badge.className).toContain("badge");
            expect(badge.className).toContain("badge-sm");
            expect(badge.className).toContain(expectedClass);
        },
    );
});

// ---------------------------------------------------------------------------
// FindingTypeBadge
// ---------------------------------------------------------------------------
describe("FindingTypeBadge", () => {
    describe("without httpStatus", () => {
        const cases: Array<{
            type: FindingType;
            expectedClass: string;
            expectedLabel: string;
        }> = [
            { type: "TIMEOUT", expectedClass: "badge-warning", expectedLabel: "TIMEOUT" },
            { type: "SERVER_ERROR", expectedClass: "badge-error", expectedLabel: "SERVER ERROR" },
            { type: "LATENCY_REGRESSION", expectedClass: "badge-info", expectedLabel: "LATENCY REGRESSION" },
            { type: "REGEX_MATCH", expectedClass: "badge-secondary", expectedLabel: "REGEX MATCH" },
        ];

        it.each(cases)(
            "renders $type with $expectedClass and label \"$expectedLabel\"",
            ({ type, expectedClass, expectedLabel }) => {
                render(<FindingTypeBadge type={type} />, ROUTE);
                const badge = screen.getByText(expectedLabel);
                expect(badge.className).toContain(expectedClass);
            },
        );
    });

    describe("with httpStatus for SERVER_ERROR", () => {
        it("shows the HTTP status code instead of the label", () => {
            render(<FindingTypeBadge type="SERVER_ERROR" httpStatus={502} />, ROUTE);
            const badge = screen.getByText("502");
            expect(badge).toBeInTheDocument();
            expect(badge.className).toContain("badge-error");
            // The text "SERVER ERROR" must not appear
            expect(screen.queryByText("SERVER ERROR")).not.toBeInTheDocument();
        });
    });

    describe("with httpStatus undefined for SERVER_ERROR", () => {
        it("shows \"SERVER ERROR\" label", () => {
            render(<FindingTypeBadge type="SERVER_ERROR" />, ROUTE);
            expect(screen.getByText("SERVER ERROR")).toBeInTheDocument();
        });
    });
});

// ---------------------------------------------------------------------------
// FindingStatusBadge
// ---------------------------------------------------------------------------
describe("FindingStatusBadge", () => {
    const cases: Array<{
        status: FindingStatus;
        expectedClass: string;
    }> = [
        { status: "UNCONFIRMED", expectedClass: "badge-ghost" },
        { status: "CONFIRMED", expectedClass: "badge-success" },
    ];

    it.each(cases)(
        "renders $status with $expectedClass",
        ({ status, expectedClass }) => {
            render(<FindingStatusBadge status={status} />, ROUTE);
            const badge = screen.getByText(status);
            expect(badge.className).toContain(expectedClass);
        },
    );
});

// ---------------------------------------------------------------------------
// MethodBadge
// ---------------------------------------------------------------------------
describe("MethodBadge", () => {
    const cases: Array<{
        method: string;
        expectedClass: string;
    }> = [
        { method: "GET", expectedClass: "badge-info" },
        { method: "POST", expectedClass: "badge-success" },
        { method: "DELETE", expectedClass: "badge-error" },
        { method: "PATCH", expectedClass: "badge-warning" },
        { method: "PUT", expectedClass: "badge-warning" },
        { method: "OPTIONS", expectedClass: "badge-ghost" },
    ];

    it.each(cases)(
        "renders $method with $expectedClass",
        ({ method, expectedClass }) => {
            render(<MethodBadge method={method} />, ROUTE);
            const badge = screen.getByText(method);
            expect(badge.className).toContain(expectedClass);
        },
    );
});

// ---------------------------------------------------------------------------
// TruncatedEndpoint
// ---------------------------------------------------------------------------
describe("TruncatedEndpoint", () => {
    it("renders the value", () => {
        render(<TruncatedEndpoint value="/api/v1/users/12345/profile" />, ROUTE);
        expect(
            screen.getByText("/api/v1/users/12345/profile"),
        ).toBeInTheDocument();
    });

    it("has a title attribute set to the value", () => {
        render(<TruncatedEndpoint value="/api/v1/users/12345/profile" />, ROUTE);
        const el = screen.getByText("/api/v1/users/12345/profile");
        expect(el).toHaveAttribute("title", "/api/v1/users/12345/profile");
    });

    it("has the expected CSS classes", () => {
        render(<TruncatedEndpoint value="/some/path" />, ROUTE);
        const el = screen.getByText("/some/path");
        expect(el.className).toContain("font-mono");
        expect(el.className).toContain("text-xs");
        expect(el.className).toContain("block");
        expect(el.className).toContain("overflow-hidden");
        expect(el.className).toContain("whitespace-nowrap");
        expect(el.className).toContain("text-ellipsis");
    });
});
