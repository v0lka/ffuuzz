import type {
    RecordingSession,
    ImportResult,
    TreeOrigin,
    DeleteByPrefixResponse,
} from "@/types/api";
import {
    get,
    post,
    del,
    delWithParams,
    ApiClientError,
    parseApiErrorResponse,
    buildQueryString,
    API_BASE,
} from "./client";

export async function listRecordings(params?: {
    limit?: number;
    offset?: number;
    host?: string;
    path_prefix?: string;
}): Promise<RecordingSession[]> {
    const result = await get<RecordingSession[]>(`${API_BASE}/recordings`, params);
    return result ?? [];
}

export function getRecording(
    id: string,
    includeEntries = false,
    maxBodyBytes = 0,
): Promise<RecordingSession> {
    return get<RecordingSession>(`${API_BASE}/recordings/${id}`, {
        include_entries: includeEntries ? "true" : "false",
        max_body_bytes: maxBodyBytes || undefined,
    });
}

export function importRecordings(
    sessions: RecordingSession[],
): Promise<ImportResult> {
    return post<ImportResult>(`${API_BASE}/recordings/import`, { sessions });
}

export function deleteRecording(id: string): Promise<void> {
    return del(`${API_BASE}/recordings/${id}`);
}

export function getRecordingsTree(): Promise<TreeOrigin[]> {
    return get<TreeOrigin[]>(`${API_BASE}/recordings/tree`);
}

export function deleteRecordingsByPrefix(params: {
    scheme: string;
    host: string;
    port: number;
    path_prefix?: string;
}): Promise<DeleteByPrefixResponse> {
    return delWithParams<DeleteByPrefixResponse>(
        `${API_BASE}/recordings/by-prefix`,
        params,
    );
}

export async function exportRecordings(params?: {
    host?: string;
    path_prefix?: string;
}): Promise<void> {
    const url = `${API_BASE}/recordings/export${buildQueryString(params)}`;

    const res = await fetch(url);
    if (!res.ok) {
        throw new ApiClientError(
            res.status,
            await parseApiErrorResponse(res, "EXPORT_FAILED"),
        );
    }
    const blob = await res.blob();
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "recordings-export.json";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(a.href);
}
