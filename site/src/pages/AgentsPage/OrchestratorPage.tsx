import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useLocation, useNavigate } from "react-router";
import { getErrorStatus } from "#/api/errors";
import { createChat, orchestratorChat } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AgentChatPageNotFoundView } from "./AgentChatPageView";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { buildAgentChatPath } from "./utils/navigation";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const OrchestratorPage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const { experiments } = useDashboard();
	const orchestratorEnabled = experiments.includes("chat-orchestrator");
	const { permissions } = useAuthenticated();
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const orchestratorChatQuery = useQuery({
		...orchestratorChat(),
		enabled: orchestratorEnabled,
	});
	const createMutation = useMutation(createChat(queryClient));

	const handleCreateChat = async ({
		message,
		fileIDs,
		model,
		reasoningEffort,
		mcpServerIds,
		organizationId,
	}: CreateChatOptions) => {
		const content: TypesGen.ChatInputPart[] = [];
		if (message.trim()) {
			content.push({ type: "text", text: message });
		}
		if (fileIDs) {
			for (const fileID of fileIDs) {
				content.push({ type: "file", file_id: fileID });
			}
		}

		const createdChat = await createMutation.mutateAsync({
			organization_id: organizationId,
			content,
			orchestrator: true,
			mcp_server_ids:
				mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
			...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
		});

		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
		}
		navigate({
			pathname: buildAgentChatPath({ chatId: createdChat.id }),
			search: location.search,
		});
	};

	if (!orchestratorEnabled) {
		return <AgentChatPageNotFoundView />;
	}

	if (orchestratorChatQuery.isLoading) {
		return <Loader />;
	}

	if (orchestratorChatQuery.data) {
		return (
			<Navigate
				to={{
					pathname: buildAgentChatPath({
						chatId: orchestratorChatQuery.data.id,
					}),
					search: location.search,
				}}
				replace
			/>
		);
	}

	if (getErrorStatus(orchestratorChatQuery.error) !== 404) {
		return <ErrorAlert error={orchestratorChatQuery.error} />;
	}

	return (
		<>
			<AgentPageHeader />
			<AgentCreateForm
				onCreateChat={handleCreateChat}
				isCreating={createMutation.isPending}
				createError={createMutation.error}
				canCreateChat={permissions.createChat}
				canConfigureAgentSetup={permissions.editDeploymentConfig}
				aiGatewayDisabled={aiGatewayDisabled}
				workspaceCount={undefined}
				workspaceOptions={[]}
				workspacesError={undefined}
				isWorkspacesLoading={false}
				hideWorkspacePicker
				hidePlanMode
				title="Orchestrator"
				description="A single persistent chat with no workspace that can spawn chats and review your existing chats."
				placeholder="Ask the orchestrator to start, check on, or summarize your chats..."
			/>
		</>
	);
};

export default OrchestratorPage;
