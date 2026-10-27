import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	disconnectMCPServerOAuth2,
	mcpServerConfigs,
	mcpServerConfigsKey,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useMCPOAuthFlow } from "../../hooks/useMCPOAuthFlow";
import { AgentSettingsMCPConnectionsPageView } from "./AgentSettingsMCPConnectionsPageView";

const AgentSettingsMCPConnectionsPage: React.FC = () => {
	const { user } = useAuthenticated();
	const location = useLocation();
	const queryClient = useQueryClient();
	// The dashboard lists every organization the user can read, but chats
	// only run in organizations the user is a member of.
	const memberOrganizations = useDashboard().organizations.filter(
		(organization) => user.organization_ids.includes(organization.id),
	);
	// Manage connections in the chat menu passes the chat's organization.
	const [selectedOrganizationId, setSelectedOrganizationId] = useState<
		string | undefined
	>(
		() =>
			(location.state as { organizationId?: string } | null)?.organizationId,
	);
	const organization =
		memberOrganizations.find(({ id }) => id === selectedOrganizationId) ??
		memberOrganizations.find(({ is_default }) => is_default) ??
		memberOrganizations.at(0);
	const organizationId = organization?.id ?? "";

	const serversQuery = useQuery({
		...mcpServerConfigs(organizationId),
		enabled: organizationId !== "",
	});
	const servers = serversQuery.data;

	const { connectingServerId, connect } = useMCPOAuthFlow({
		organizationId,
		onAuthComplete: () => {
			void queryClient.invalidateQueries({
				queryKey: mcpServerConfigsKey(organizationId),
			});
		},
		onFlowSuccess: (serverId) => {
			const server = servers?.find(({ id }) => id === serverId);
			if (server) {
				toast.success(`Connected ${server.display_name}.`);
			}
		},
	});

	const [disconnectTarget, setDisconnectTarget] =
		useState<TypesGen.MCPServerConfig | null>(null);
	const disconnectMutation = useMutation(
		disconnectMCPServerOAuth2(queryClient),
	);
	const handleConfirmDisconnect = () => {
		if (!disconnectTarget) {
			return;
		}
		const name = disconnectTarget.display_name;
		disconnectMutation.mutate(disconnectTarget.id, {
			onSuccess: (response) => {
				setDisconnectTarget(null);
				if (response.token_revocation_error) {
					toast.warning(`Disconnected ${name}.`, {
						description: response.token_revocation_error,
					});
				} else {
					toast.success(`Disconnected ${name}.`);
				}
			},
			onError: (error) => {
				toast.error(getErrorMessage(error, `Failed to disconnect ${name}.`));
			},
		});
	};

	return (
		<AgentSettingsMCPConnectionsPageView
			organizations={memberOrganizations}
			organization={organization}
			onOrganizationChange={({ id }) => setSelectedOrganizationId(id)}
			servers={servers}
			error={serversQuery.error}
			isLoading={serversQuery.isLoading}
			isRetrying={serversQuery.isFetching}
			onRetry={() => {
				void serversQuery.refetch();
			}}
			connectingServerId={connectingServerId}
			onConnect={(server) => connect(server.id)}
			onDisconnect={setDisconnectTarget}
			disconnectTarget={disconnectTarget}
			isDisconnecting={disconnectMutation.isPending}
			onConfirmDisconnect={handleConfirmDisconnect}
			onCancelDisconnect={() => setDisconnectTarget(null)}
		/>
	);
};

export default AgentSettingsMCPConnectionsPage;
