import { useCallback, useState } from "react";
import type {
    ConfigResponse,
    ConfigUpdateRequest,
    ConfigValidationError,
    TLSConfigResponse,
    CertCacheConfigResponse,
    LLMConfigResponse,
} from "@/types/api";
import { ErrorAlert } from "@/components/ErrorAlert";
import { ApiClientError } from "@/api/client";

interface Props {
    config: ConfigResponse;
    isPending: boolean;
    submitError: Error | null;
    onSave: (values: ConfigUpdateRequest) => void;
}

// --- Validation helpers ---

function isValidDuration(v: string): boolean {
    const s = v.trim();
    if (!s) return false;
    return /^\d+(\.\d+)?(ns|us|µs|ms|s|m|h)$/.test(s);
}

// --- Field component ---

interface FieldProps {
    label: string;
    helper?: string;
    error?: string;
    children: React.ReactNode;
}

function ConfigField({ label, helper, error, children }: FieldProps) {
    return (
        <div className="form-control w-full">
            <label className="label py-1">
                <span className="label-text">{label}</span>
                {helper && <span className="label-text-alt opacity-60">{helper}</span>}
            </label>
            {children}
            {error && <span className="text-error text-xs mt-1">{error}</span>}
        </div>
    );
}

// --- Input sub-components ---

function TextField({
    value,
    onChange,
    onBlur,
    placeholder,
    error,
}: {
    value: string;
    onChange: (v: string) => void;
    onBlur?: () => void;
    placeholder?: string;
    error?: string;
}) {
    return (
        <input
            type="text"
            className={`input input-bordered w-full ${error ? "input-error" : ""}`}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onBlur={onBlur}
            placeholder={placeholder}
        />
    );
}

function PasswordField({
    value,
    onChange,
    onBlur,
    error,
    apiKeySet,
}: {
    value: string;
    onChange: (v: string) => void;
    onBlur?: () => void;
    error?: string;
    apiKeySet: boolean;
}) {
    return (
        <input
            type="password"
            className={`input input-bordered w-full ${error ? "input-error" : ""}`}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onBlur={onBlur}
            placeholder={apiKeySet ? "Key is set (enter new value to change)" : "Enter API key"}
        />
    );
}

function NumberField({
    value,
    onChange,
    onBlur,
    error,
    min,
}: {
    value: number;
    onChange: (v: number) => void;
    onBlur?: () => void;
    error?: string;
    min?: number;
}) {
    return (
        <input
            type="number"
            className={`input input-bordered w-40 ${error ? "input-error" : ""}`}
            value={value}
            min={min}
            onChange={(e) => onChange(parseInt(e.target.value, 10) || 0)}
            onBlur={onBlur}
        />
    );
}

function ToggleField({
    checked,
    onChange,
}: {
    checked: boolean;
    onChange: (v: boolean) => void;
}) {
    return (
        <input
            type="checkbox"
            className="toggle toggle-sm"
            checked={checked}
            onChange={(e) => onChange(e.target.checked)}
        />
    );
}

function SelectField({
    value,
    onChange,
    onBlur,
    options,
    error,
    placeholder,
}: {
    value: string;
    onChange: (v: string) => void;
    onBlur?: () => void;
    options: { value: string; label: string }[];
    error?: string;
    placeholder?: string;
}) {
    return (
        <select
            className={`select select-bordered w-full max-w-xs ${error ? "select-error" : ""}`}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onBlur={onBlur}
        >
            {placeholder && <option value="">{placeholder}</option>}
            {options.map((opt) => (
                <option key={opt.value} value={opt.value}>
                    {opt.label}
                </option>
            ))}
        </select>
    );
}

// --- Section wrapper ---

function ConfigSection({ title, children }: { title: string; children: React.ReactNode }) {
    return (
        <div className="card bg-base-200 shadow-sm">
            <div className="card-body p-4 md:p-6">
                <h2 className="card-title text-lg">{title}</h2>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">{children}</div>
            </div>
        </div>
    );
}

// --- Main component ---

type Errors = Record<string, string>;

export default function ConfigForm({ config, isPending, submitError, onSave }: Props) {
    // Flat state (keeps things simple)
    const [apiAddress, setAPIAddress] = useState(config.api_address);
    const [proxyAddress, setProxyAddress] = useState(config.proxy_address);
    const [databaseURI, setDatabaseURI] = useState(config.database_uri);
    const [artifactDir, setArtifactDir] = useState(config.artifact_dir);
    const [reqTimeout, setReqTimeout] = useState(config.req_timeout);
    const [shutdownTimeout, setShutdownTimeout] = useState(config.shutdown_timeout);
    const [workers, setWorkers] = useState(config.workers);
    const [rps, setRPS] = useState(config.rps);
    const [maxBodyBytes, setMaxBodyBytes] = useState(config.max_body_bytes);
    const [tlsSkipVerify, setTLSSkipVerify] = useState(config.tls_skip_verify);
    const [tls, setTLS] = useState<TLSConfigResponse>(config.tls);
    const [certCache, setCertCache] = useState<CertCacheConfigResponse>(config.cert_cache);
    const [llm, setLLM] = useState<LLMConfigResponse>(config.llm);
    const [llmApiKey, setLLMApiKey] = useState("");
    const llmApiKeySet = config.llm.api_key !== "";

    const [errors, setErrors] = useState<Errors>({});

    const setFieldError = useCallback((field: string, msg: string) => {
        setErrors((prev) => {
            const next = { ...prev };
            if (msg) {
                next[field] = msg;
            } else {
                delete next[field];
            }
            return next;
        });
    }, []);

    // Validation on blur
    const validateDuration = useCallback(
        (field: string, value: string) => {
            if (!isValidDuration(value)) {
                setFieldError(field, "Must be a valid duration (e.g. 3s, 500ms, 1m)");
            } else {
                setFieldError(field, "");
            }
        },
        [setFieldError],
    );

    const validatePositive = useCallback(
        (field: string, value: number) => {
            if (value <= 0) {
                setFieldError(field, "Must be a positive integer");
            } else {
                setFieldError(field, "");
            }
        },
        [setFieldError],
    );

    const handleSave = () => {
        const req: ConfigUpdateRequest = {
            api_address: apiAddress,
            proxy_address: proxyAddress,
            database_uri: databaseURI,
            artifact_dir: artifactDir,
            req_timeout: reqTimeout,
            shutdown_timeout: shutdownTimeout,
            workers,
            rps,
            max_body_bytes: maxBodyBytes,
            tls_skip_verify: tlsSkipVerify,
            tls,
            cert_cache: certCache,
            llm: {
                ...llm,
                api_key: llmApiKeySet && !llmApiKey ? undefined : llmApiKey,
            },
        };
        onSave(req);
    };

    // Parse server-side validation errors and map to form fields
    const serverErrors = (() => {
        if (
            submitError instanceof ApiClientError &&
            submitError.code === "VALIDATION_FAILED"
        ) {
            try {
                const body = JSON.parse(submitError.message) as ConfigValidationError;
                if (body.fields) {
                    return body.fields;
                }
            } catch {
                // not JSON
            }
        }
        return null;
    })();

    // Merge server errors into local errors display
    const displayErrors = { ...errors };
    if (serverErrors) {
        for (const fe of serverErrors) {
            displayErrors[fe.field] = fe.message;
        }
    }

    const hasErrors = Object.keys(errors).length > 0;

    return (
        <form
            onSubmit={(e) => {
                e.preventDefault();
                handleSave();
            }}
            className="space-y-6"
        >
            {/* Global errors */}
            {submitError && !serverErrors && (
                <ErrorAlert
                    message={
                        submitError instanceof ApiClientError
                            ? submitError.message
                            : "Failed to save configuration"
                    }
                    requestId={
                        submitError instanceof ApiClientError
                            ? submitError.requestId
                            : undefined
                    }
                />
            )}
            {serverErrors && (
                <ErrorAlert message="Some configuration values are invalid. See fields below for details." />
            )}

            {/* Server */}
            <ConfigSection title="Server Addresses">
                <ConfigField label="API Address" helper="e.g. :8081 or 0.0.0.0:8081">
                    <TextField value={apiAddress} onChange={setAPIAddress} />
                </ConfigField>
                <ConfigField label="Proxy Address" helper="e.g. :8080 or 0.0.0.0:8080">
                    <TextField value={proxyAddress} onChange={setProxyAddress} />
                </ConfigField>
            </ConfigSection>

            {/* Database */}
            <ConfigSection title="Database">
                <ConfigField label="PostgreSQL URI" helper="postgres://user:pass@host:port/db?sslmode=disable">
                    <TextField
                        value={databaseURI}
                        onChange={setDatabaseURI}
                        placeholder="postgres://ffuuzz:ffuuzz@localhost:5432/ffuuzz?sslmode=disable"
                    />
                </ConfigField>
            </ConfigSection>

            {/* Storage */}
            <ConfigSection title="Storage">
                <ConfigField label="Artifact Directory" helper="Directory for campaign artifacts">
                    <TextField value={artifactDir} onChange={setArtifactDir} />
                </ConfigField>
            </ConfigSection>

            {/* Performance */}
            <ConfigSection title="Performance">
                <ConfigField
                    label="Request Timeout"
                    helper="Per HTTP request timeout"
                    error={displayErrors.req_timeout}
                >
                    <TextField
                        value={reqTimeout}
                        onChange={setReqTimeout}
                        onBlur={() => validateDuration("req_timeout", reqTimeout)}
                        error={displayErrors.req_timeout}
                    />
                </ConfigField>
                <ConfigField
                    label="Shutdown Timeout"
                    helper="Graceful shutdown wait"
                    error={displayErrors.shutdown_timeout}
                >
                    <TextField
                        value={shutdownTimeout}
                        onChange={setShutdownTimeout}
                        onBlur={() => validateDuration("shutdown_timeout", shutdownTimeout)}
                        error={displayErrors.shutdown_timeout}
                    />
                </ConfigField>
                <ConfigField
                    label="Workers"
                    helper="Concurrent fuzzing workers (> 0)"
                    error={displayErrors.workers}
                >
                    <NumberField
                        value={workers}
                        onChange={setWorkers}
                        onBlur={() => validatePositive("workers", workers)}
                        error={displayErrors.workers}
                        min={1}
                    />
                </ConfigField>
                <ConfigField
                    label="RPS"
                    helper="Global rate limit (> 0)"
                    error={displayErrors.rps}
                >
                    <NumberField
                        value={rps}
                        onChange={setRPS}
                        onBlur={() => validatePositive("rps", rps)}
                        error={displayErrors.rps}
                        min={1}
                    />
                </ConfigField>
                <ConfigField
                    label="Max Body Bytes"
                    helper="Max response body to record (> 0)"
                    error={displayErrors.max_body_bytes}
                >
                    <NumberField
                        value={maxBodyBytes}
                        onChange={setMaxBodyBytes}
                        onBlur={() => validatePositive("max_body_bytes", maxBodyBytes)}
                        error={displayErrors.max_body_bytes}
                        min={1}
                    />
                </ConfigField>
            </ConfigSection>

            {/* TLS */}
            <ConfigSection title="TLS Settings">
                <ConfigField label="Skip TLS Verify" helper="Skip upstream certificate verification">
                    <ToggleField checked={tlsSkipVerify} onChange={setTLSSkipVerify} />
                </ConfigField>
                <ConfigField
                    label="Min TLS Version"
                    error={displayErrors["tls.min_version"]}
                >
                    <SelectField
                        value={tls.min_version}
                        onChange={(v) =>
                            setTLS((prev) => ({ ...prev, min_version: v as TLSConfigResponse["min_version"] }))
                        }
                        options={[
                            { value: "1.2", label: "TLS 1.2" },
                            { value: "1.3", label: "TLS 1.3" },
                        ]}
                        error={displayErrors["tls.min_version"]}
                    />
                </ConfigField>
                <ConfigField
                    label="Handshake Timeout"
                    error={displayErrors["tls.handshake_timeout"]}
                >
                    <TextField
                        value={tls.handshake_timeout}
                        onChange={(v) => setTLS((prev) => ({ ...prev, handshake_timeout: v }))}
                        onBlur={() =>
                            validateDuration("tls.handshake_timeout", tls.handshake_timeout)
                        }
                        error={displayErrors["tls.handshake_timeout"]}
                    />
                </ConfigField>
                <ConfigField label="Disable Session Tickets" helper="RFC 5077 forward secrecy">
                    <ToggleField
                        checked={tls.disable_session_tickets}
                        onChange={(v) => setTLS((prev) => ({ ...prev, disable_session_tickets: v }))}
                    />
                </ConfigField>
            </ConfigSection>

            {/* Certificate Cache */}
            <ConfigSection title="Certificate Cache">
                <ConfigField
                    label="Max Entries"
                    helper="LRU cache size (> 0)"
                    error={displayErrors["cert_cache.max_entries"]}
                >
                    <NumberField
                        value={certCache.max_entries}
                        onChange={(v) => setCertCache((prev) => ({ ...prev, max_entries: v }))}
                        onBlur={() => validatePositive("cert_cache.max_entries", certCache.max_entries)}
                        error={displayErrors["cert_cache.max_entries"]}
                        min={1}
                    />
                </ConfigField>
                <ConfigField label="Memory Only" helper="Keep certs in memory (no disk I/O)">
                    <ToggleField
                        checked={certCache.memory_only}
                        onChange={(v) => setCertCache((prev) => ({ ...prev, memory_only: v }))}
                    />
                </ConfigField>
                <ConfigField label="Cert Directory" helper="Path for persisted certificates">
                    <TextField
                        value={certCache.cert_dir}
                        onChange={(v) => setCertCache((prev) => ({ ...prev, cert_dir: v }))}
                    />
                </ConfigField>
            </ConfigSection>

            {/* LLM */}
            <ConfigSection title="LLM-Assisted Triage">
                <ConfigField label="Enabled" helper="Enable AI-powered finding classification">
                    <ToggleField
                        checked={llm.enabled}
                        onChange={(v) => setLLM((prev) => ({ ...prev, enabled: v }))}
                    />
                </ConfigField>
                <ConfigField
                    label="Provider"
                    error={displayErrors["llm.provider"]}
                >
                    <SelectField
                        value={llm.provider}
                        onChange={(v) => setLLM((prev) => ({ ...prev, provider: v }))}
                        options={[
                            { value: "anthropic", label: "Anthropic (Claude)" },
                            { value: "openai", label: "OpenAI (GPT)" },
                        ]}
                        placeholder="(provider default)"
                        error={displayErrors["llm.provider"]}
                    />
                </ConfigField>
                <ConfigField
                    label="API Key"
                    helper={llmApiKeySet ? "A key is currently set" : "Provider API key"}
                    error={displayErrors["llm.api_key"]}
                >
                    <PasswordField
                        value={llmApiKey}
                        onChange={setLLMApiKey}
                        apiKeySet={llmApiKeySet}
                        error={displayErrors["llm.api_key"]}
                    />
                </ConfigField>
                <ConfigField label="Base URL" helper="Override API endpoint (for proxies/local models)">
                    <TextField
                        value={llm.base_url}
                        onChange={(v) => setLLM((prev) => ({ ...prev, base_url: v }))}
                        placeholder="https://api.openai.com"
                    />
                </ConfigField>
                <ConfigField label="Model" helper="e.g. gpt-4o, claude-sonnet-4">
                    <TextField
                        value={llm.model}
                        onChange={(v) => setLLM((prev) => ({ ...prev, model: v }))}
                        placeholder="(provider default)"
                    />
                </ConfigField>
                <ConfigField
                    label="Max Tokens"
                    helper="Max tokens in LLM response (> 0)"
                    error={displayErrors["llm.max_tokens"]}
                >
                    <NumberField
                        value={llm.max_tokens}
                        onChange={(v) => setLLM((prev) => ({ ...prev, max_tokens: v }))}
                        onBlur={() => validatePositive("llm.max_tokens", llm.max_tokens)}
                        error={displayErrors["llm.max_tokens"]}
                        min={1}
                    />
                </ConfigField>
                <ConfigField
                    label="Timeout"
                    helper="Per LLM API call timeout"
                    error={displayErrors["llm.timeout"]}
                >
                    <TextField
                        value={llm.timeout}
                        onChange={(v) => setLLM((prev) => ({ ...prev, timeout: v }))}
                        onBlur={() => validateDuration("llm.timeout", llm.timeout)}
                        error={displayErrors["llm.timeout"]}
                    />
                </ConfigField>
            </ConfigSection>

            {/* Save */}
            <div className="flex items-center gap-4">
                <button
                    type="submit"
                    className="btn btn-primary"
                    disabled={isPending || hasErrors}
                >
                    {isPending && <span className="loading loading-spinner loading-xs" />}
                    Save Configuration
                </button>
                <p className="text-sm opacity-60">
                    Changes are written to the <code className="text-xs">.env</code> file and take effect on next server restart.
                </p>
            </div>
        </form>
    );
}
