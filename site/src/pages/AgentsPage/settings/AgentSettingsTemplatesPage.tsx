import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatTemplateAllowlist,
	updateChatTemplateAllowlist,
} from "#/api/queries/chats";
import { templates } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsTemplatesPageView } from "./AgentSettingsTemplatesPageView";

const AgentSettingsTemplatesPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	const templatesQuery = useQuery(templates());
	const allowlistQuery = useQuery(chatTemplateAllowlist());
	const saveAllowlistMutation = useMutation(
		updateChatTemplateAllowlist(queryClient),
	);

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsTemplatesPageView
				templatesQuery={templatesQuery}
				allowlistQuery={allowlistQuery}
				saveAllowlistMutation={saveAllowlistMutation}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsTemplatesPage;
