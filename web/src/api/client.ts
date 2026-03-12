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
        let apiErr: APIError;
        try {
            apiErr = (await res.json()) as APIError;
        } catch {
            apiErr = {
                error: "UNKNOWN",
                message: res.statusText,
                request_id: "",
            };
        }
        throw new ApiClientError(res.status, apiErr);
    }

    return (await res.json()) as T;
}

export function get<T>(
    url: string,
    params?: Record<string, string | number | undefined>,
): Promise<T> {
    const searchParams = new URLSearchParams();
    if (params) {
        for (const [key, value] of Object.entries(params)) {
            if (value !== undefined && value !== "") {
                searchParams.set(key, String(value));
            }
        }
    }
    const qs = searchParams.toString();
    const fullUrl = qs ? `${url}?${qs}` : url;
    return request<T>("GET", fullUrl);
}

export function post<T>(url: string, body?: unknown): Promise<T> {
    return request<T>("POST", url, body);
}

export function del(url: string): Promise<void> {
    return request<void>("DELETE", url);
}

export function delWithParams<T = void>(
    url: string,
    params?: Record<string, string | number | undefined>,
): Promise<T> {
    const searchParams = new URLSearchParams();
    if (params) {
        for (const [key, value] of Object.entries(params)) {
            if (value !== undefined && value !== "") {
                searchParams.set(key, String(value));
            }
        }
    }
    const qs = searchParams.toString();
    const fullUrl = qs ? `${url}?${qs}` : url;
    return request<T>("DELETE", fullUrl);
}

export { ApiClientError };
