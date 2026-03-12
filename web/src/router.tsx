import { createBrowserRouter } from "react-router-dom";
import Layout from "@/components/Layout";
import DashboardPage from "@/pages/DashboardPage";
import RecordingsPage from "@/pages/RecordingsPage";
import RecordingDetailPage from "@/pages/RecordingDetailPage";
import CampaignsPage from "@/pages/CampaignsPage";
import CampaignCreatePage from "@/pages/CampaignCreatePage";
import CampaignDetailPage from "@/pages/CampaignDetailPage";
import FindingsPage from "@/pages/FindingsPage";
import FindingDetailPage from "@/pages/FindingDetailPage";
import NotFoundPage from "@/pages/NotFoundPage";

export const router = createBrowserRouter(
    [
        {
            element: <Layout />,
            children: [
                { index: true, element: <DashboardPage /> },
                { path: "recordings", element: <RecordingsPage /> },
                { path: "recordings/:id", element: <RecordingDetailPage /> },
                { path: "campaigns", element: <CampaignsPage /> },
                { path: "campaigns/new", element: <CampaignCreatePage /> },
                { path: "campaigns/:id", element: <CampaignDetailPage /> },
                { path: "findings", element: <FindingsPage /> },
                { path: "findings/:id", element: <FindingDetailPage /> },
                { path: "*", element: <NotFoundPage /> },
            ],
        },
    ],
    { basename: "/ui" },
);
