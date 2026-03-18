import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useCreateCampaign, useRecordings } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import type {
    CreateCampaignRequest,
    CampaignLimits,
    MutationConfig,
    AnomalyConfig,
    TriageConfig,
} from "@/types/api";

const defaults: CreateCampaignRequest = {
    name: "",
    recording_ids: [],
    config: {
        target: {},
        limits: {
            workers: 8,
            rps: 50,
            max_tests: 0,
            duration_sec: 0,
            req_timeout_ms: 3000,
        },
        mutations: {
            path_query: true,
            headers: true,
            json_body: true,
            params: true,
            sequence: false,
            intensity: 0.6,
        },
        anomaly: {
            detect_5xx: true,
            latency_multiplier: 3.0,
            regex_patterns: [],
        },
        triage: {
            confirm_runs: 3,
            enable_minimization: true,
        },
    },
};

export default function CampaignCreatePage() {
    const navigate = useNavigate();
    const createMutation = useCreateCampaign();
    const recordings = useRecordings({ limit: 100 });
    const [form, setForm] = useState<CreateCampaignRequest>(defaults);
    const [regexText, setRegexText] = useState("");

    const updateLimits = (key: keyof CampaignLimits, value: number) => {
        setForm((prev) => ({
            ...prev,
            config: {
                ...prev.config,
                limits: { ...prev.config.limits, [key]: value },
            },
        }));
    };

    const updateMutations = <K extends keyof MutationConfig>(key: K, value: MutationConfig[K]) => {
        setForm((prev) => ({
            ...prev,
            config: {
                ...prev.config,
                mutations: { ...prev.config.mutations, [key]: value },
            },
        }));
    };

    const updateAnomaly = <K extends keyof AnomalyConfig>(key: K, value: AnomalyConfig[K]) => {
        setForm((prev) => ({
            ...prev,
            config: {
                ...prev.config,
                anomaly: { ...prev.config.anomaly, [key]: value },
            },
        }));
    };

    const updateTriage = <K extends keyof TriageConfig>(key: K, value: TriageConfig[K]) => {
        setForm((prev) => ({
            ...prev,
            config: {
                ...prev.config,
                triage: { ...prev.config.triage, [key]: value },
            },
        }));
    };

    const toggleRecording = (id: string) => {
        setForm((prev) => {
            const ids = prev.recording_ids.includes(id)
                ? prev.recording_ids.filter((r) => r !== id)
                : [...prev.recording_ids, id];
            return { ...prev, recording_ids: ids };
        });
    };

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        const req = structuredClone(form);
        req.config.anomaly.regex_patterns = regexText
            .split("\n")
            .map((s) => s.trim())
            .filter(Boolean);

        createMutation.mutate(req, {
            onSuccess: (campaign) => {
                navigate(`/campaigns/${campaign.id}`);
            },
        });
    };

    if (recordings.isLoading) return <LoadingSpinner />;

    return (
        <div className="max-w-2xl space-y-6">
            <h1 className="text-2xl font-bold">New Campaign</h1>

            {createMutation.isError && (
                <ErrorAlert message={createMutation.error.message} />
            )}

            <form onSubmit={handleSubmit} className="space-y-6">
                {/* Basic */}
                <div className="card bg-base-200">
                    <div className="card-body p-4 space-y-3">
                        <h3 className="font-semibold">Basic Info</h3>
                        <label className="label">
                            <span className="label-text">Name</span>
                        </label>
                        <input
                            type="text"
                            className="input input-bordered w-full"
                            required
                            value={form.name}
                            onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
                        />

                        <label className="label">
                            <span className="label-text">Recordings</span>
                        </label>
                        <div className="max-h-40 overflow-y-auto border border-base-300 rounded-box p-2 space-y-1">
                            {recordings.data && recordings.data.length > 0 ? (
                                recordings.data.map((r) => (
                                    <label
                                        key={r.id}
                                        className="flex items-center gap-2 cursor-pointer"
                                    >
                                        <input
                                            type="checkbox"
                                            className="checkbox checkbox-sm"
                                            checked={form.recording_ids.includes(r.id)}
                                            onChange={() => toggleRecording(r.id)}
                                        />
                                        <span className="font-mono text-xs">{r.id.slice(0, 8)}</span>
                                        <span className="text-xs opacity-60">
                                            {r.target.host}:{r.target.port}{r.target.path} ({r.entry_count ?? 0} entries)
                                        </span>
                                    </label>
                                ))
                            ) : (
                                <p className="text-xs opacity-50">No recordings available</p>
                            )}
                        </div>
                    </div>
                </div>

                {/* Limits */}
                <div className="card bg-base-200">
                    <div className="card-body p-4 space-y-3">
                        <h3 className="font-semibold">Limits</h3>
                        <div className="grid grid-cols-2 gap-3">
                            {([
                                { label: "Workers", key: "workers" as const, val: form.config.limits.workers },
                                { label: "RPS", key: "rps" as const, val: form.config.limits.rps },
                                { label: "Max Tests (0=unlimited)", key: "max_tests" as const, val: form.config.limits.max_tests },
                                { label: "Duration (sec, 0=unlimited)", key: "duration_sec" as const, val: form.config.limits.duration_sec },
                                { label: "Request Timeout (ms)", key: "req_timeout_ms" as const, val: form.config.limits.req_timeout_ms },
                            ]).map((f) => (
                                <div key={f.key}>
                                    <label className="label"><span className="label-text text-xs">{f.label}</span></label>
                                    <input
                                        type="number"
                                        className="input input-bordered input-sm w-full"
                                        value={f.val}
                                        onChange={(e) => updateLimits(f.key, Number(e.target.value))}
                                    />
                                </div>
                            ))}
                        </div>
                    </div>
                </div>

                {/* Mutations */}
                <div className="card bg-base-200">
                    <div className="card-body p-4 space-y-3">
                        <h3 className="font-semibold">Mutations</h3>
                        <div className="flex flex-wrap gap-4">
                            {([
                                { label: "Path/Query", key: "path_query" as const, val: form.config.mutations.path_query },
                                { label: "Headers", key: "headers" as const, val: form.config.mutations.headers },
                                { label: "JSON Body", key: "json_body" as const, val: form.config.mutations.json_body },
                                { label: "Params", key: "params" as const, val: form.config.mutations.params },
                                { label: "Sequence", key: "sequence" as const, val: form.config.mutations.sequence },
                            ]).map((f) => (
                                <label key={f.key} className="flex items-center gap-2 cursor-pointer">
                                    <input
                                        type="checkbox"
                                        className="toggle toggle-sm"
                                        checked={f.val}
                                        onChange={(e) => updateMutations(f.key, e.target.checked)}
                                    />
                                    <span className="text-sm">{f.label}</span>
                                </label>
                            ))}
                        </div>
                        <div>
                            <label className="label mb-1"><span className="label-text text-xs">Intensity: {form.config.mutations.intensity.toFixed(1)}</span></label>
                            <input
                                type="range"
                                min="0"
                                max="1"
                                step="0.1"
                                className="range range-sm"
                                value={form.config.mutations.intensity}
                                onChange={(e) => updateMutations("intensity", Number(e.target.value))}
                            />
                        </div>
                    </div>
                </div>

                {/* Anomaly */}
                <div className="card bg-base-200">
                    <div className="card-body p-4 space-y-3">
                        <h3 className="font-semibold">Anomaly Detection</h3>
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                className="toggle toggle-sm"
                                checked={form.config.anomaly.detect_5xx}
                                onChange={(e) => updateAnomaly("detect_5xx", e.target.checked)}
                            />
                            <span className="text-sm">Detect 5xx errors</span>
                        </label>
                        <div>
                            <label className="label"><span className="label-text text-xs">Latency Multiplier</span></label>
                            <input
                                type="number"
                                step="0.5"
                                className="input input-bordered input-sm w-32"
                                value={form.config.anomaly.latency_multiplier}
                                onChange={(e) => updateAnomaly("latency_multiplier", Number(e.target.value))}
                            />
                        </div>
                        <div>
                            <label className="label"><span className="label-text text-xs">Regex Patterns (one per line)</span></label>
                            <textarea
                                className="textarea textarea-bordered w-full text-xs font-mono"
                                rows={3}
                                value={regexText}
                                onChange={(e) => setRegexText(e.target.value)}
                            />
                        </div>
                    </div>
                </div>

                {/* Triage */}
                <div className="card bg-base-200">
                    <div className="card-body p-4 space-y-3">
                        <h3 className="font-semibold">Triage</h3>
                        <div>
                            <label className="label"><span className="label-text text-xs">Confirmation Runs</span></label>
                            <input
                                type="number"
                                className="input input-bordered input-sm w-32"
                                value={form.config.triage.confirm_runs}
                                onChange={(e) => updateTriage("confirm_runs", Number(e.target.value))}
                            />
                        </div>
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                className="toggle toggle-sm"
                                checked={form.config.triage.enable_minimization}
                                onChange={(e) => updateTriage("enable_minimization", e.target.checked)}
                            />
                            <span className="text-sm">Enable minimization</span>
                        </label>
                    </div>
                </div>

                {/* Submit */}
                <button
                    type="submit"
                    className="btn btn-primary w-full"
                    disabled={
                        !form.name ||
                        form.recording_ids.length === 0 ||
                        createMutation.isPending
                    }
                >
                    {createMutation.isPending ? (
                        <span className="loading loading-spinner loading-sm" />
                    ) : (
                        "Create Campaign"
                    )}
                </button>
            </form>
        </div>
    );
}
