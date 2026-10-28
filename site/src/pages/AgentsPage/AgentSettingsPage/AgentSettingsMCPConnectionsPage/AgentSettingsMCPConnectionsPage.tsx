import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation } from "react-router";
import { getErrorMessage } from "#/api/errors";
import {
	disconnectMCPServerOAuth2,
	invalidateMCPServerConfigQueries,
	mcpServerConfigs,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getDefaultOrganizationId,
	getMemberOrganizations,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { useMCPOAuthFlow } from "../../hooks/useMCPOAuthFlow";
import { AgentSettingsMCPConnectionsPageView } from "./AgentSettingsMCPConnectionsPageView";

/** Reads the organization a link into this page asked to show first. */
const getStateOrganizationId = (state: unknown): string | undefined =>
	typeof state === "object" &&
	state !== null &&
	"organizationId" in state &&
	typeof state.organizationId === "string"
		? state.organizationId
		: undefined;

const AgentSettingsMCPConnectionsPage: React.FC = () => {
	const { user } = useAuthenticated();
	const location = useLocation();
	const queryClient = useQueryClient();
	const memberOrganizations = getMemberOrganizations(
		useDashboard().organizations,
		user,
	);
	const [selectedOrganizationId, setSelectedOrganizationId] = useState(() =>
		getStateOrganizationId(location.state),
	);
	const defaultOrganizationId = getDefaultOrganizationId(memberOrganizations);
	const organization =
		memberOrganizations.find(({ id }) => id === selectedOrganizationId) ??
		memberOrganizations.find(({ id }) => id === defaultOrganizationId) ??
		memberOrganizations.at(0);
	const organizationId = organization?.id ?? "";

	const serversQuery = useQuery({
		...mcpServerConfigs(organizationId),
		enabled: organizationId !== "",
	});
	const servers = serversQuery.data;

	const { connectingServerId, connect } = useMCPOAuthFlow({
		organizationId,
		// A sign-in can finish after the picker leaves its organization,
		// so every organization's list refreshes.
		onAuthComplete: () => {
			void invalidateMCPServerConfigQueries(queryClient);
		},
		onFlowSuccess: (server) => {
			toast.success(`Connected ${server.display_name}.`);
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
			onConnect={connect}
			onDisconnect={setDisconnectTarget}
			disconnectTarget={disconnectTarget}
			isDisconnecting={disconnectMutation.isPending}
			onConfirmDisconnect={handleConfirmDisconnect}
			onCancelDisconnect={() => setDisconnectTarget(null)}
		/>
	);
};

export default AgentSettingsMCPConnectionsPage;
