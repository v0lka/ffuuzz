import type {
    Finding,
    ArtifactPayload,
    LLMAnalysis,
    ReproduceResponse,
} from "@/types/api";
import { get, post, API_BASE } from "./client";

export async function listFindings(params?: {
    campaign_id?: string;
    type?: string;
    status?: string;
    since?: string;
    limit?: number;
    offset?: number;
}): Promise<Finding[]> {
    const result = await get<Finding[]>(`${API_BASE}/findings`, params);
    return result ?? [];
}

export function getFinding(id: string): Promise<Finding> {
    return get<Finding>(`${API_BASE}/findings/${id}`);
}

export function getFindingArtifact(id: string): Promise<ArtifactPayload> {
    return get<ArtifactPayload>(`${API_BASE}/findings/${id}/artifact`);
}

export function reproduceFinding(
    id: string,
    runs = 3,
): Promise<ReproduceResponse> {
    return post<ReproduceResponse>(`${API_BASE}/findings/${id}/reproduce`, {
        runs,
    });
}

export function analyzeFinding(id: string): Promise<LLMAnalysis> {
    return post<LLMAnalysis>(`${API_BASE}/findings/${id}/analyze`);
}
