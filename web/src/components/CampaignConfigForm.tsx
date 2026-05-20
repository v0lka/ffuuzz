import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import type {
    AnomalyConfig,
    CampaignConfig,
    CampaignLimits,
    MutationConfig,
    RecordingSession,
    TriageConfig,
} from "@/types/api";

// Operator metadata for each mutator category.
// Used to render per-operator toggles in the UI.
const OPERATOR_META: Record<string, { label: string; desc: string }[]> = {
    path_query: [
        { label: "Path segment", desc: "Insert/delete/rename path segments" },
        { label: "Query param", desc: "Manipulate query parameters" },
        { label: "Reserved chars", desc: "Inject URI-reserved characters" },
        { label: "Percent encoding", desc: "Inject malformed percent sequences" },
        { label: "Slash manipulation", desc: "Toggle/add/encode slashes" },
        { label: "Long value", desc: "Append long path segments" },
    ],
    headers: [
        { label: "Add", desc: "Add random header" },
        { label: "Remove", desc: "Remove random header" },
        { label: "Duplicate", desc: "Duplicate header value" },
        { label: "Long value", desc: "Replace with oversized value" },
        { label: "Dict substitute", desc: "Substitute from dictionary" },
        { label: "Conflicting", desc: "Inject conflicting headers" },
    ],
    json_body: [
        { label: "Type substitute", desc: "Change value type" },
        { label: "Object key", desc: "Modify object keys" },
        { label: "Array mutation", desc: "Mutate JSON arrays" },
        { label: "Boundary values", desc: "Inject numeric extremes" },
        { label: "Depth stress", desc: "Deeply nested objects" },
        { label: "String mutation", desc: "Inject fuzz payloads" },
    ],
    params: [
        { label: "String mutation", desc: "Inject fuzz payloads into params" },
    ],
    sequence: [
        { label: "Drop", desc: "Remove one exchange" },
        { label: "Duplicate", desc: "Duplicate one exchange" },
        { label: "Swap", desc: "Swap adjacent exchanges" },
        { label: "Per-step", desc: "Primitive mutation on one exchange" },
    ],
    primitive: [
        { label: "Bitflip", desc: "Flip a single bit" },
        { label: "Byteflip", desc: "Flip all bits in a byte" },
        { label: "Arithmetic", desc: "Add/subtract from a byte" },
        { label: "Interesting", desc: "Replace with boundary values" },
        { label: "Block op", desc: "Insert/delete/duplicate blocks" },
        { label: "Splice", desc: "Cross with response body" },
    ],
};

// Operator JSON field names for each category.
const OPERATOR_KEYS: Record<string, keyof MutationConfig> = {
    path_query: "uri_operators",
    headers: "header_operators",
    json_body: "json_operators",
    params: "param_operators",
    primitive: "primitive_operators",
    sequence: "sequence_operators",
};

// Operator name lists for each category (matches Go all*Ops).
const OPERATOR_NAMES: Record<string, string[]> = {
    path_query: ["path_segment", "query_param", "reserved_inject", "percent_encoding", "slash_manipulation", "long_value"],
    headers: ["add", "remove", "duplicate", "long_value", "dict_substitute", "conflicting"],
    json_body: ["type_substitute", "object_key", "array_mutation", "boundary_values", "depth_stress", "string_mutation"],
    params: ["string_mutation"],
    primitive: ["bitflip", "byteflip", "arith", "interesting", "block_op", "splice"],
    sequence: ["drop", "duplicate", "swap", "perstep"],
};

export const defaultCampaignConfig: CampaignConfig = {
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
};

export interface CampaignFormValues {
    name: string;
    recording_ids: string[];
    config: CampaignConfig;
}

interface Props {
    initialName?: string;
    initialRecordingIDs?: string[];
    initialConfig?: CampaignConfig;
    recordings: RecordingSession[];
    submitLabel: string;
    isPending: boolean;
    submitError?: Error | null;
    onSubmit: (values: CampaignFormValues) => void;
}

export function CampaignConfigForm({
    initialName = "",
    initialRecordingIDs = [],
    initialConfig = defaultCampaignConfig,
    recordings,
    submitLabel,
    isPending,
    submitError,
    onSubmit,
}: Props) {
    const [name, setName] = useState(initialName);
    const [recordingIDs, setRecordingIDs] = useState<string[]>(initialRecordingIDs);
    const [cfg, setCfg] = useState<CampaignConfig>(initialConfig);
    const [regexText, setRegexText] = useState(
        (initialConfig.anomaly.regex_patterns ?? []).join("\n"),
    );

    const updateLimits = (key: keyof CampaignLimits, value: number) => {
        setCfg((prev) => ({
            ...prev,
            limits: { ...prev.limits, [key]: value },
        }));
    };

    const updateMutations = <K extends keyof MutationConfig>(key: K, value: MutationConfig[K]) => {
        setCfg((prev) => ({
            ...prev,
            mutations: { ...prev.mutations, [key]: value },
        }));
    };

    const toggleOperator = (categoryKey: string, opName: string) => {
        const configKey = OPERATOR_KEYS[categoryKey];
        if (!configKey) return;
        setCfg((prev) => {
            const currentList = (prev.mutations[configKey] as string[] | undefined) ?? [];
            let newList: string[];
            if (currentList.includes(opName)) {
                newList = currentList.filter((o) => o !== opName);
            } else {
                const allOps = OPERATOR_NAMES[categoryKey] ?? [];
                const baseList = currentList.length > 0 ? currentList : allOps;
                newList = baseList.filter((o) => o !== opName);
            }
            if (newList.length > 0) {
                const allOps = OPERATOR_NAMES[categoryKey];
                if (allOps && newList.length === allOps.length && allOps.every((o) => newList.includes(o))) {
                    newList = [];
                }
            }
            return {
                ...prev,
                mutations: {
                    ...prev.mutations,
                    [configKey]: newList.length > 0 ? newList : undefined,
                },
            };
        });
    };

    const isOperatorEnabled = (categoryKey: string, opName: string): boolean => {
        const configKey = OPERATOR_KEYS[categoryKey];
        if (!configKey) return true;
        const currentList = (cfg.mutations[configKey] as string[] | undefined);
        if (!currentList || currentList.length === 0) return true;
        return currentList.includes(opName);
    };

    const renderOperatorToggles = (categoryKey: string) => {
        const ops = OPERATOR_META[categoryKey];
        const names = OPERATOR_NAMES[categoryKey];
        if (!ops || !names) return null;
        return (
            <div className="ml-6 mt-2 space-y-1">
                {ops.map((op, i) => {
                    const opName = names[i];
                    if (!opName) return null;
                    return (
                    <label key={opName} className="flex items-center gap-2 cursor-pointer py-0.5">
                        <input
                            type="checkbox"
                            className="checkbox checkbox-xs"
                            checked={isOperatorEnabled(categoryKey, opName)}
                            onChange={() => toggleOperator(categoryKey, opName)}
                        />
                        <span className="text-xs">{op.label}</span>
                        <span className="text-xs opacity-40">{op.desc}</span>
                    </label>
                    );
                })}
            </div>
        );
    };

    const updateAnomaly = <K extends keyof AnomalyConfig>(key: K, value: AnomalyConfig[K]) => {
        setCfg((prev) => ({
            ...prev,
            anomaly: { ...prev.anomaly, [key]: value },
        }));
    };

    const updateTriage = <K extends keyof TriageConfig>(key: K, value: TriageConfig[K]) => {
        setCfg((prev) => ({
            ...prev,
            triage: { ...prev.triage, [key]: value },
        }));
    };

    const toggleRecording = (rid: string) => {
        setRecordingIDs((prev) =>
            prev.includes(rid) ? prev.filter((r) => r !== rid) : [...prev, rid],
        );
    };

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        onSubmit({
            name,
            recording_ids: recordingIDs,
            config: {
                ...cfg,
                anomaly: {
                    ...cfg.anomaly,
                    regex_patterns: regexText
                        .split("\n")
                        .map((s) => s.trim())
                        .filter(Boolean),
                },
            },
        });
    };

    return (
        <form onSubmit={handleSubmit} className="space-y-6">
            {submitError && <ErrorAlert message={submitError.message} />}

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
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                    />

                    <label className="label">
                        <span className="label-text">Recordings</span>
                    </label>
                    <div className="max-h-40 overflow-y-auto border border-base-300 rounded-box p-2 space-y-1">
                        {recordings.length > 0 ? (
                            recordings.map((r) => (
                                <label
                                    key={r.id}
                                    className="flex items-center gap-2 cursor-pointer"
                                >
                                    <input
                                        type="checkbox"
                                        className="checkbox checkbox-sm"
                                        checked={recordingIDs.includes(r.id)}
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
                            { label: "Workers", key: "workers" as const, val: cfg.limits.workers },
                            { label: "RPS", key: "rps" as const, val: cfg.limits.rps },
                            { label: "Max Tests (0=unlimited)", key: "max_tests" as const, val: cfg.limits.max_tests },
                            { label: "Duration (sec, 0=unlimited)", key: "duration_sec" as const, val: cfg.limits.duration_sec },
                            { label: "Request Timeout (ms)", key: "req_timeout_ms" as const, val: cfg.limits.req_timeout_ms },
                        ]).map((f) => (
                            <div key={f.key}>
                                <label className="label"><span className="label-text text-xs mr-2 mb-1">{f.label}</span></label>
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

                    {/* Main category toggles */}
                    <div className="flex flex-wrap gap-4">
                        {([
                            { label: "Path/Query", key: "path_query" as const, val: cfg.mutations.path_query },
                            { label: "Headers", key: "headers" as const, val: cfg.mutations.headers },
                            { label: "JSON Body", key: "json_body" as const, val: cfg.mutations.json_body },
                            { label: "Params", key: "params" as const, val: cfg.mutations.params },
                            { label: "Sequence", key: "sequence" as const, val: cfg.mutations.sequence },
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
                        {/* Primitive is always enabled (fallback) but can be configured */}
                        <label className="flex items-center gap-2 opacity-50">
                            <input type="checkbox" className="toggle toggle-sm" checked disabled />
                            <span className="text-sm">Primitive (auto)</span>
                        </label>
                    </div>

                    {/* Per-operator toggles under each category */}
                    {cfg.mutations.path_query && (
                        <details className="mt-2" open>
                            <summary className="text-xs font-medium cursor-pointer opacity-70 hover:opacity-100">URI operators</summary>
                            {renderOperatorToggles("path_query")}
                        </details>
                    )}
                    {cfg.mutations.headers && (
                        <details className="mt-2" open>
                            <summary className="text-xs font-medium cursor-pointer opacity-70 hover:opacity-100">Header operators</summary>
                            {renderOperatorToggles("headers")}
                        </details>
                    )}
                    {cfg.mutations.json_body && (
                        <details className="mt-2" open>
                            <summary className="text-xs font-medium cursor-pointer opacity-70 hover:opacity-100">JSON operators</summary>
                            {renderOperatorToggles("json_body")}
                        </details>
                    )}
                    {cfg.mutations.params && (
                        <details className="mt-2" open>
                            <summary className="text-xs font-medium cursor-pointer opacity-70 hover:opacity-100">Param operators</summary>
                            {renderOperatorToggles("params")}
                        </details>
                    )}
                    {cfg.mutations.sequence && (
                        <details className="mt-2" open>
                            <summary className="text-xs font-medium cursor-pointer opacity-70 hover:opacity-100">Sequence operators</summary>
                            {renderOperatorToggles("sequence")}
                        </details>
                    )}
                    {/* Primitive operators always shown since primitive is always enabled */}
                    <details className="mt-2" open>
                        <summary className="text-xs font-medium cursor-pointer opacity-70 hover:opacity-100">Primitive operators</summary>
                        {renderOperatorToggles("primitive")}
                    </details>

                    <div>
                        <label className="label mb-1"><span className="label-text text-xs mr-2 mb-1">Intensity: {cfg.mutations.intensity.toFixed(1)}</span></label>
                        <input
                            type="range"
                            min="0"
                            max="1"
                            step="0.1"
                            className="range range-sm"
                            value={cfg.mutations.intensity}
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
                            checked={cfg.anomaly.detect_5xx}
                            onChange={(e) => updateAnomaly("detect_5xx", e.target.checked)}
                        />
                        <span className="text-sm">Detect 5xx errors</span>
                    </label>
                    <div>
                        <label className="label"><span className="label-text text-xs mr-2 mb-1">Latency Multiplier</span></label>
                        <input
                            type="number"
                            step="0.5"
                            className="input input-bordered input-sm w-32"
                            value={cfg.anomaly.latency_multiplier}
                            onChange={(e) => updateAnomaly("latency_multiplier", Number(e.target.value))}
                        />
                    </div>
                    <div>
                        <label className="label"><span className="label-text text-xs mr-2 mb-1">Regex Patterns (one per line)</span></label>
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
                        <label className="label"><span className="label-text text-xs mr-2 mb-1">Confirmation Runs</span></label>
                        <input
                            type="number"
                            className="input input-bordered input-sm w-32"
                            value={cfg.triage.confirm_runs}
                            onChange={(e) => updateTriage("confirm_runs", Number(e.target.value))}
                        />
                    </div>
                    <label className="flex items-center gap-2 cursor-pointer">
                        <input
                            type="checkbox"
                            className="toggle toggle-sm"
                            checked={cfg.triage.enable_minimization}
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
                disabled={!name || recordingIDs.length === 0 || isPending}
            >
                {isPending ? (
                    <span className="loading loading-spinner loading-sm" />
                ) : (
                    submitLabel
                )}
            </button>
        </form>
    );
}
