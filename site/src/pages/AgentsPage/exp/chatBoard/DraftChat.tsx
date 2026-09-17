import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { createChat } from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "../../components/AgentCreateForm";
import { lastModelConfigIDStorageKey } from "./assistants";

interface DraftChatProps {
	/** Labels that place the chat on the board, sent with the create request. */
	readonly labels: Record<string, string>;
	/** Appended to the first message as a fenced block when present. A note may itself contain a three-backtick fence. */
	readonly context: string | undefined;
	readonly onCreated: (chatId: string) => void;
}

/**
 * The regular create form inside a board window. Submit is a copy of
 * AgentCreatePage.handleCreateChat plus labels and context, so the chat is
 * born on the board and the regular system prompt applies unchanged.
 */
export const DraftChat: FC<DraftChatProps> = ({
	labels,
	context,
	onCreated,
}) => {
	const queryClient = useQueryClient();
	const { permissions } = useAuthenticated();
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));

	const handleCreateChat = async ({
		message,
		fileIDs,
		workspaceId,
		model,
		reasoningEffort,
		mcpServerIds,
		organizationId,
		planMode,
	}: CreateChatOptions) => {
		const text = [
			message.trim() ? message : "",
			context ? `\`\`\`\`\n${context}\n\`\`\`\`` : "",
		]
			.filter(Boolean)
			.join("\n\n");
		const content: TypesGen.ChatInputPart[] = [];
		if (text) {
			content.push({ type: "text", text });
		}
		for (const fileID of fileIDs ?? []) {
			content.push({ type: "file", file_id: fileID });
		}
		const createdChat = await createMutation.mutateAsync({
			organization_id: organizationId,
			content,
			workspace_id: workspaceId,
			mcp_server_ids:
				mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
			plan_mode: planMode === "plan" ? "plan" : undefined,
			client_type: "ui",
			labels,
			...(model ? { model_config_id: model } : {}),
			...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
		});
		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
		}
		onCreated(createdChat.id);
	};

	return (
		<AgentCreateForm
			onCreateChat={handleCreateChat}
			isCreating={createMutation.isPending}
			createError={createMutation.error}
			canCreateChat={permissions.createChat}
			canConfigureAgentSetup={permissions.editDeploymentConfig}
			aiGatewayDisabled={aiGatewayDisabled}
			workspaceCount={workspacesQuery.data?.count}
			workspaceOptions={workspacesQuery.data?.workspaces ?? []}
			workspacesError={workspacesQuery.error}
			isWorkspacesLoading={workspacesQuery.isLoading}
		/>
	);
};
