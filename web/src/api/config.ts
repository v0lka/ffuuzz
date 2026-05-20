import { get, put, API_BASE } from "./client";
import type { ConfigResponse, ConfigUpdateRequest, ConfigSaveResponse } from "@/types/api";

export function getConfig(): Promise<ConfigResponse> {
    return get<ConfigResponse>(`${API_BASE}/config`);
}

export function updateConfig(req: ConfigUpdateRequest): Promise<ConfigSaveResponse> {
    return put<ConfigSaveResponse>(`${API_BASE}/config`, req);
}
