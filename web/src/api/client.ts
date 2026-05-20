import type { APIError } from "@/types/api";

export const API_BASE = "/api/v1";

class ApiClientError extends Error {
    public status: number;
    public code: string;
    public requestId: string;

    constructor(status: number, body: APIError) {
        super(body.message);
        this.name = "ApiClientError";
        this.status = status;
        this.code = body.error;
        this.requestId = body.request_id;
    }
}

async function parseApiErrorResponse(
    res: Response,
    defaultError = "UNKNOWN",
): Promise<APIError> {
    try {
        return (await res.json()) as APIError;
    } catch {
        return { error: defaultError, message: res.statusText, request_id: "" };
    }
}

function buildQueryString(
    params?: Record<string, string | number | undefined>,
): string {
    if (!params) return "";
    const sp = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
        if (value !== undefined && value !== "") {
            sp.set(key, String(value));
        }
    }
    const qs = sp.toString();
    return qs ? `?${qs}` : "";
}

async function request<T>(
    method: string,
    url: string,
    body?: unknown,
): Promise<T> {
    const headers: Record<string, string> = {};
    if (body !== undefined) {
        headers["Content-Type"] = "application/json";
    }

    const res = await fetch(url, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
    });

    if (res.status === 204) {
        return undefined as T;
    }

    if (!res.ok) {
        throw new ApiClientError(
            res.status,
            await parseApiErrorResponse(res),
        );
    }

    return (await res.json()) as T;
}

export function get<T>(
    url: string,
    params?: Record<string, string | number | undefined>,
): Promise<T> {
    return request<T>("GET", `${url}${buildQueryString(params)}`);
}

export function post<T>(url: string, body?: unknown): Promise<T> {
    return request<T>("POST", url, body);
}

export function del(url: string): Promise<void> {
    return request<void>("DELETE", url);
}

export function put<T>(url: string, body?: unknown): Promise<T> {
    return request<T>("PUT", url, body);
}

export function delWithParams<T = void>(
    url: string,
    params?: Record<string, string | number | undefined>,
): Promise<T> {
    return request<T>("DELETE", `${url}${buildQueryString(params)}`);
}

export { ApiClientError, parseApiErrorResponse, buildQueryString };
