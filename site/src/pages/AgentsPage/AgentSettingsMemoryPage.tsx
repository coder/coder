import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate } from "react-router";
import {
	chatPersonalMemorySettings,
	updateChatPersonalMemorySettings,
} from "#/api/queries/chatUserMemories";
import {
	getDefaultOrganizationId,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { AgentSettingsMemoryPageView } from "./AgentSettingsMemoryPageView";

const AgentSettingsMemoryPage: FC = () => {
	const { experiments, organizations } = useDashboard();
	const queryClient = useQueryClient();
	const defaultOrganizationId = getDefaultOrganizationId(organizations);
	const [selectedOrganizationId, setSelectedOrganizationId] = useState(
		defaultOrganizationId,
	);
	const selectedOrganization =
		organizations.find(
			(organization) => organization.id === selectedOrganizationId,
		) ??
		organizations.find(
			(organization) => organization.id === defaultOrganizationId,
		) ??
		organizations[0];
	const settingsQuery = useQuery({
		...chatPersonalMemorySettings(),
		enabled: experiments.includes("chat-projects"),
	});
	const updateSettingsMutation = useMutation(
		updateChatPersonalMemorySettings(queryClient),
	);

	if (!experiments.includes("chat-projects")) {
		return <Navigate to="/agents" replace />;
	}

	return (
		<AgentSettingsMemoryPageView
			organizations={organizations}
			selectedOrganization={selectedOrganization}
			settings={settingsQuery.data}
			settingsError={settingsQuery.error}
			isLoadingSettings={settingsQuery.isLoading}
			isSavingSettings={updateSettingsMutation.isPending}
			isSaveSettingsError={updateSettingsMutation.isError}
			onSelectOrganization={(organization) =>
				setSelectedOrganizationId(organization.id)
			}
			onSaveSettings={(request) => updateSettingsMutation.mutate(request)}
		/>
	);
};

export default AgentSettingsMemoryPage;
