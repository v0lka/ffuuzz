import { useParams, useNavigate } from "react-router-dom";
import {
    useCampaign,
    useCampaignConfig,
    useRecordings,
    useUpdateCampaign,
} from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorAlert } from "@/components/ErrorAlert";
import {
    CampaignConfigForm,
    type CampaignFormValues,
} from "@/components/CampaignConfigForm";
import type { UpdateCampaignRequest } from "@/types/api";

function isEditable(status: string) {
    return ["CREATED", "STOPPED", "FINISHED", "FAILED"].includes(status);
}

export default function CampaignEditPage() {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const safeId = id ?? "";

    const campaign = useCampaign(safeId);
    const config = useCampaignConfig(safeId);
    const recordings = useRecordings({ limit: 100 });
    const updateMutation = useUpdateCampaign();

    if (!id) return <ErrorAlert message="Missing campaign ID" />;
    if (campaign.isLoading || config.isLoading || recordings.isLoading) {
        return <LoadingSpinner />;
    }
    if (campaign.error || config.error) {
        return <ErrorAlert message="Failed to load campaign data" />;
    }
    if (!campaign.data || !config.data) {
        return <ErrorAlert message="Campaign not found" />;
    }
    if (!isEditable(campaign.data.status)) {
        return (
            <ErrorAlert
                message={`Cannot edit campaign in state: ${campaign.data.status}`}
            />
        );
    }

    const handleSubmit = (values: CampaignFormValues) => {
        const req: UpdateCampaignRequest = {
            name: values.name,
            recording_ids: values.recording_ids,
            config: values.config,
        };
        updateMutation.mutate(
            { id: safeId, req },
            {
                onSuccess: () => {
                    navigate(`/campaigns/${safeId}`);
                },
            },
        );
    };

    return (
        <div className="max-w-2xl space-y-6">
            <h1 className="text-2xl font-bold">Edit Campaign</h1>
            <CampaignConfigForm
                initialName={campaign.data.name}
                initialRecordingIDs={campaign.data.recording_ids ?? []}
                initialConfig={config.data}
                recordings={recordings.data ?? []}
                submitLabel="Save Changes"
                isPending={updateMutation.isPending}
                submitError={updateMutation.isError ? updateMutation.error : null}
                onSubmit={handleSubmit}
            />
        </div>
    );
}
