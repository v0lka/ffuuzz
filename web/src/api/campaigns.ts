import type {
    Campaign,
    CampaignConfig,
    CampaignStats,
    CreateCampaignRequest,
    Finding,
    AddRecordingsResponse,
} from "@/types/api";
import { get, post, API_BASE } from "./client";

export function listCampaigns(params?: {
    status?: string;
    limit?: number;
    offset?: number;
}): Promise<Campaign[]> {
    return get<Campaign[]>(`${API_BASE}/campaigns`, params);
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

export function getCampaignFindings(
    id: string,
    params?: {
        type?: string;
        status?: string;
        since?: string;
        limit?: number;
        offset?: number;
    },
): Promise<Finding[]> {
    return get<Finding[]>(`${API_BASE}/campaigns/${id}/findings`, params);
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
