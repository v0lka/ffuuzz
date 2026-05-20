import { useState } from "react";
import { useConfig, useUpdateConfig } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import ConfigForm from "@/components/ConfigForm";
import type { ConfigUpdateRequest } from "@/types/api";

export default function ConfigPage() {
    const config = useConfig();
    const updateMutation = useUpdateConfig();
    const [saved, setSaved] = useState(false);

    if (config.isLoading) return <LoadingSpinner />;
    if (config.error) return <ErrorAlert message="Failed to load configuration" />;
    if (!config.data) return <ErrorAlert message="No configuration data" />;

    const handleSave = (values: ConfigUpdateRequest) => {
        setSaved(false);
        updateMutation.mutate(values, {
            onSuccess: () => {
                setSaved(true);
            },
        });
    };

    return (
        <div className="max-w-2xl space-y-6">
            <div>
                <h1 className="text-2xl font-bold">Configuration</h1>
                <p className="text-sm opacity-60 mt-1">
                    Edit the <code className="text-xs">.env</code> file. Changes take effect on next server restart.
                </p>
            </div>

            {saved && (
                <div role="alert" className="alert alert-success">
                    <svg
                        xmlns="http://www.w3.org/2000/svg"
                        className="h-6 w-6 shrink-0 stroke-current"
                        fill="none"
                        viewBox="0 0 24 24"
                    >
                        <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth="2"
                            d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                        />
                    </svg>
                    <span>
                        Configuration saved. Changes take effect on next restart.
                    </span>
                </div>
            )}

            <ConfigForm
                config={config.data}
                isPending={updateMutation.isPending}
                submitError={updateMutation.error}
                onSave={handleSave}
            />
        </div>
    );
}
