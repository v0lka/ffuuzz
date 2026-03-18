import type { HealthResponse } from "@/types/api";
import { get } from "./client";

export function getHealth(): Promise<HealthResponse> {
    return get<HealthResponse>("/healthz");
}
