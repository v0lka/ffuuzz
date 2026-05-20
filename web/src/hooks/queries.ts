import { useEffect, useRef } from "react";
import {
    useQuery,
    useMutation,
    useQueryClient,
} from "@tanstack/react-query";
import type {
    Campaign,
    CampaignConfig,
    CampaignStats,
    ConfigResponse,
    CreateCampaignRequest,
    UpdateCampaignRequest,
    Finding,
    ArtifactPayload,
    LLMAnalysis,
    RecordingSession,
    TreeOrigin,
    HealthResponse,
} from "@/types/api";
import * as recordingsApi from "@/api/recordings";
import * as campaignsApi from "@/api/campaigns";
import * as findingsApi from "@/api/findings";
import * as healthApi from "@/api/health";
import * as configApi from "@/api/config";
import type { ConfigUpdateRequest } from "@/types/api";

export const queryKeys = {
    recordings: {
        all: ["recordings"] as const,
        list: <T extends Record<string, unknown> = Record<string, unknown>>(params?: T) =>
            ["recordings", "list", params] as const,
        detail: (id: string) => ["recordings", "detail", id] as const,
        tree: ["recordings", "tree"] as const,
    },
    campaigns: {
        all: ["campaigns"] as const,
        list: <T extends Record<string, unknown> = Record<string, unknown>>(params?: T) =>
            ["campaigns", "list", params] as const,
        detail: (id: string) => ["campaigns", "detail", id] as const,
        stats: (id: string) => ["campaigns", "stats", id] as const,
        config: (id: string) => ["campaigns", "config", id] as const,
        findings: <T extends Record<string, unknown> = Record<string, unknown>>(id: string, params?: T) =>
            ["campaigns", "findings", id, params] as const,
    },
    findings: {
        all: ["findings"] as const,
        list: <T extends Record<string, unknown> = Record<string, unknown>>(params?: T) =>
            ["findings", "list", params] as const,
        detail: (id: string) => ["findings", "detail", id] as const,
        artifact: (id: string) => ["findings", "artifact", id] as const,
    },
    health: ["health"] as const,
    config: ["config"] as const,
};

export function useRecordings(params?: {
    limit?: number;
    offset?: number;
    host?: string;
    path_prefix?: string;
}) {
    return useQuery<RecordingSession[]>({
        queryKey: queryKeys.recordings.list(params),
        queryFn: () => recordingsApi.listRecordings(params),
        refetchInterval: 1_000,
    });
}

export function useRecording(id: string, includeEntries = false) {
    return useQuery<RecordingSession>({
        queryKey: queryKeys.recordings.detail(id),
        queryFn: () =>
            recordingsApi.getRecording(id, includeEntries, 65536),
        enabled: !!id,
    });
}

export function useRecordingsTree() {
    return useQuery<TreeOrigin[]>({
        queryKey: queryKeys.recordings.tree,
        queryFn: () => recordingsApi.getRecordingsTree(),
        refetchInterval: 1_000,
    });
}

export function useImportRecordings() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (sessions: RecordingSession[]) =>
            recordingsApi.importRecordings(sessions),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.recordings.all });
        },
    });
}

export function useDeleteRecording() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => recordingsApi.deleteRecording(id),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.recordings.all });
        },
    });
}

export function useDeleteRecordingsByPrefix() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (params: {
            scheme: string;
            host: string;
            port: number;
            path_prefix?: string;
        }) => recordingsApi.deleteRecordingsByPrefix(params),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.recordings.all });
        },
    });
}

export function useCampaigns(params?: {
    status?: string;
    limit?: number;
    offset?: number;
}) {
    return useQuery<Campaign[]>({
        queryKey: queryKeys.campaigns.list(params),
        queryFn: () => campaignsApi.listCampaigns(params),
        staleTime: 10_000,
    });
}

export function useCampaign(id: string) {
    return useQuery<Campaign>({
        queryKey: queryKeys.campaigns.detail(id),
        queryFn: () => campaignsApi.getCampaign(id),
        enabled: !!id,
    });
}

export function useCampaignConfig(id: string) {
    return useQuery<CampaignConfig>({
        queryKey: queryKeys.campaigns.config(id),
        queryFn: () => campaignsApi.getCampaignConfig(id),
        enabled: !!id,
    });
}

export function useCampaignStats(id: string, enabled = true, refetchInterval: number | false = 5_000) {
    return useQuery<CampaignStats>({
        queryKey: queryKeys.campaigns.stats(id),
        queryFn: () => campaignsApi.getCampaignStats(id),
        enabled: !!id && enabled,
        refetchInterval,
    });
}

export function useCampaignFindings(
    id: string,
    params?: {
        type?: string;
        status?: string;
        limit?: number;
        offset?: number;
    },
    refetchInterval: number | false = 2_000,
) {
    return useQuery<Finding[]>({
        queryKey: queryKeys.campaigns.findings(id, params),
        queryFn: () => campaignsApi.getCampaignFindings(id, params),
        enabled: !!id,
        refetchInterval,
    });
}

export function useCreateCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (req: CreateCampaignRequest) =>
            campaignsApi.createCampaign(req),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
        },
    });
}

export function useStartCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => campaignsApi.startCampaign(id),
        onSuccess: (_data, id) => {
            void qc.invalidateQueries({
                queryKey: queryKeys.campaigns.detail(id),
            });
            void qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
        },
    });
}

export function useStopCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => campaignsApi.stopCampaign(id),
        onSuccess: (_data, id) => {
            void qc.invalidateQueries({
                queryKey: queryKeys.campaigns.detail(id),
            });
            void qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
        },
    });
}

export function useAddRecordingsToCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({
            campaignId,
            filter,
        }: {
            campaignId: string;
            filter: {
                scheme: string;
                host: string;
                port: number;
                path_prefix?: string;
            };
        }) => campaignsApi.addRecordingsToCampaign(campaignId, filter),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
        },
    });
}

export function useUpdateCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({ id, req }: { id: string; req: UpdateCampaignRequest }) =>
            campaignsApi.updateCampaign(id, req),
        onSuccess: (_data, { id }) => {
            void qc.invalidateQueries({
                queryKey: queryKeys.campaigns.detail(id),
            });
            void qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
            void qc.invalidateQueries({
                queryKey: queryKeys.campaigns.config(id),
            });
        },
    });
}

export function useQuickCreateCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (req: {
            name: string;
            filter: { scheme: string; host: string; port: number; path_prefix?: string };
        }) => campaignsApi.quickCreateCampaign(req),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.campaigns.all });
        },
    });
}

export function useFindings(params?: {
    campaign_id?: string;
    type?: string;
    status?: string;
    limit?: number;
    offset?: number;
}) {
    return useQuery<Finding[]>({
        queryKey: queryKeys.findings.list(params),
        queryFn: () => findingsApi.listFindings(params),
        refetchInterval: 2_000,
    });
}

export function useFinding(id: string) {
    return useQuery<Finding>({
        queryKey: queryKeys.findings.detail(id),
        queryFn: () => findingsApi.getFinding(id),
        enabled: !!id,
    });
}

export function useFindingArtifact(id: string, enabled = true) {
    return useQuery<ArtifactPayload>({
        queryKey: queryKeys.findings.artifact(id),
        queryFn: () => findingsApi.getFindingArtifact(id),
        enabled: !!id && enabled,
    });
}

export function useReproduceFinding() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({ id, runs }: { id: string; runs?: number }) =>
            findingsApi.reproduceFinding(id, runs),
        onSuccess: (_data, { id }) => {
            void qc.invalidateQueries({
                queryKey: queryKeys.findings.detail(id),
            });
        },
    });
}

export function useAnalyzeFinding() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => findingsApi.analyzeFinding(id),
        onSuccess: (_data: LLMAnalysis, id: string) => {
            void qc.invalidateQueries({
                queryKey: queryKeys.findings.detail(id),
            });
        },
    });
}

export function useAnalyzeCampaign() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => campaignsApi.analyzeCampaign(id),
        onSuccess: (_data, id: string) => {
            void qc.invalidateQueries({
                queryKey: queryKeys.campaigns.detail(id),
            });
            void qc.invalidateQueries({
                queryKey: queryKeys.campaigns.findings(id),
            });
        },
    });
}

export function useBatchAnalysisProgress(
    campaignId: string,
    total: number,
    enabled: boolean,
) {
    const qc = useQueryClient();
    const prevAnalyzed = useRef(0);

    const query = useQuery<{ analyzed: number; total: number }>({
        queryKey: ["batchAnalysis", campaignId],
        queryFn: async () => {
            const findings = await campaignsApi.getCampaignFindings(campaignId, {
                limit: 10000,
            });
            const pending = findings.filter(
                (f) => !f.llm_analysis,
            );
            return {
                analyzed: findings.length - pending.length,
                total: total || findings.length,
            };
        },
        enabled: enabled && total > 0,
        refetchInterval: 2_000,
    });

    // Invalidate finding detail caches as batch analysis progresses,
    // so navigating to a finding page shows fresh LLM analysis results.
    useEffect(() => {
        if (query.data && query.data.analyzed > prevAnalyzed.current) {
            prevAnalyzed.current = query.data.analyzed;
            void qc.invalidateQueries({ queryKey: queryKeys.findings.all });
        }
    }, [query.data, qc]);

    return query;
}

export function useHealth() {
    return useQuery<HealthResponse>({
        queryKey: queryKeys.health,
        queryFn: () => healthApi.getHealth(),
        refetchInterval: 30_000,
    });
}

export function useConfig() {
    return useQuery<ConfigResponse>({
        queryKey: queryKeys.config,
        queryFn: () => configApi.getConfig(),
        staleTime: 60_000,
    });
}

export function useUpdateConfig() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (req: ConfigUpdateRequest) => configApi.updateConfig(req),
        onSuccess: () => {
            void qc.invalidateQueries({ queryKey: queryKeys.config });
        },
    });
}
