import { useNavigate } from "react-router-dom";
import { useCreateCampaign, useRecordings } from "@/hooks/queries";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import {
    CampaignConfigForm,
    type CampaignFormValues,
} from "@/components/CampaignConfigForm";

export default function CampaignCreatePage() {
    const navigate = useNavigate();
    const createMutation = useCreateCampaign();
    const recordings = useRecordings({ limit: 100 });

    if (recordings.isLoading) return <LoadingSpinner />;

    const handleSubmit = (values: CampaignFormValues) => {
        createMutation.mutate(values, {
            onSuccess: (campaign) => {
                navigate(`/campaigns/${campaign.id}`);
            },
        });
    };

    return (
        <div className="max-w-2xl space-y-6">
            <h1 className="text-2xl font-bold">New Campaign</h1>
            <CampaignConfigForm
                recordings={recordings.data ?? []}
                submitLabel="Create Campaign"
                isPending={createMutation.isPending}
                submitError={createMutation.isError ? createMutation.error : null}
                onSubmit={handleSubmit}
            />
        </div>
    );
}
