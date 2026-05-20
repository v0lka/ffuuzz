import type {
    Campaign,
    CampaignConfig,
    CampaignStats,
    CreateCampaignRequest,
    UpdateCampaignRequest,
    Finding,
    AddRecordingsResponse,
} from "@/types/api";
import { get, post, API_BASE } from "./client";

export async function listCampaigns(params?: {
    status?: string;
    limit?: number;
    offset?: number;
}): Promise<Campaign[]> {
    const result = await get<Campaign[]>(`${API_BASE}/campaigns`, params);
    return result ?? [];
}

export function getCampaign(id: string): Promise<Campaign> {
    return get<Campaign>(`${API_BASE}/campaigns/${id}`);
}

export function getCampaignConfig(id: string): Promise<CampaignConfig> {
    return get<CampaignConfig>(`${API_BASE}/campaigns/${id}/config`);
}

export function getCampaignStats(id: string): Promise<CampaignStats> {
    return get<CampaignStats>(`${API_BASE}/campaigns/${id}/stats`);
}

export async function getCampaignFindings(
    id: string,
    params?: {
        type?: string;
        status?: string;
        since?: string;
        limit?: number;
        offset?: number;
    },
): Promise<Finding[]> {
    const result = await get<Finding[]>(`${API_BASE}/campaigns/${id}/findings`, params);
    return result ?? [];
}

export function createCampaign(
    req: CreateCampaignRequest,
): Promise<Campaign> {
    return post<Campaign>(`${API_BASE}/campaigns`, req);
}

export function startCampaign(id: string): Promise<Campaign> {
    return post<Campaign>(`${API_BASE}/campaigns/${id}/start`);
}

export function stopCampaign(id: string): Promise<Campaign> {
    return post<Campaign>(`${API_BASE}/campaigns/${id}/stop`);
}

export function addRecordingsToCampaign(
    campaignId: string,
    filter: {
        scheme: string;
        host: string;
        port: number;
        path_prefix?: string;
    },
): Promise<AddRecordingsResponse> {
    return post<AddRecordingsResponse>(
        `${API_BASE}/campaigns/${campaignId}/recordings`,
        filter,
    );
}

export interface AnalyzeCampaignResponse {
    analyzed: number;
    message: string;
    total?: number;
}

export function analyzeCampaign(
    id: string,
): Promise<AnalyzeCampaignResponse> {
    return post<AnalyzeCampaignResponse>(
        `${API_BASE}/campaigns/${id}/analyze`,
    );
}

export function quickCreateCampaign(req: {
    name: string;
    filter: { scheme: string; host: string; port: number; path_prefix?: string };
}): Promise<Campaign> {
    return post<Campaign>(`${API_BASE}/campaigns/quick`, req);
}

export function updateCampaign(
    id: string,
    req: UpdateCampaignRequest,
): Promise<Campaign> {
    return post<Campaign>(`${API_BASE}/campaigns/${id}`, req);
}
