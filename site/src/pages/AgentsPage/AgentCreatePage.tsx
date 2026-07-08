import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	archiveChat,
	createChat,
	createChatMessageByChatId,
} from "#/api/queries/chats";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import {
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import { WebPushButton } from "./components/WebPushButton";
import { toWorkspaceFileReferencePart } from "./utils/chatInputContent";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { buildAgentChatPath } from "./utils/navigation";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const { permissions } = useAuthenticated();
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const sendFirstMessageMutation = useMutation(
		createChatMessageByChatId(queryClient),
	);
	const archiveMutation = useMutation(archiveChat(queryClient));
	// Cleanup for a shell chat whose deferred uploads or first message
	// failed. The primary failure is already toasted by the caller, so
	// this only adds the cleanup outcome.
	const archiveUnusedChat = (chatId: string) => {
		archiveMutation.mutate(chatId, {
			onError: (error) => {
				toast.error(
					getErrorMessage(error, "Failed to clean up the unused chat."),
				);
			},
		});
	};
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	const handleCreateChat = async ({
		message,
		fileIDs,
		workspaceId,
		model,
		reasoningEffort,
		mcpServerIds,
		organizationId,
		planMode,
		uploadWorkspaceFiles,
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
		const createRequest: TypesGen.CreateChatRequest = {
			organization_id: organizationId,
			// Workspace files need the chat ID to upload, so the chat is
			// created idle without content and the first message follows
			// after the uploads land.
			content: uploadWorkspaceFiles ? [] : content,
			workspace_id: workspaceId,
			mcp_server_ids:
				mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
			plan_mode: planMode === "plan" ? "plan" : undefined,
			client_type: "ui",
			...(model ? { model_config_id: model } : {}),
			...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
		};
		const createdChat = await createMutation.mutateAsync(createRequest);

		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
		}

		if (uploadWorkspaceFiles) {
			let uploaded: Awaited<ReturnType<typeof uploadWorkspaceFiles>>;
			try {
				uploaded = await uploadWorkspaceFiles(createdChat.id);
			} catch (error) {
				// The empty chat never started generating, so archiving it
				// right away is the cleanup path; retry creates a fresh one.
				archiveUnusedChat(createdChat.id);
				toast.error(
					getErrorMessage(error, "Failed to upload files to the workspace."),
				);
				throw error;
			}
			const failedCount = uploaded.filter(
				(upload) => upload.status !== "uploaded" || !upload.response,
			).length;
			if (failedCount > 0) {
				archiveUnusedChat(createdChat.id);
				toast.error(
					`${failedCount} file${failedCount > 1 ? "s" : ""} failed to upload to the workspace. Remove or retry the failed files, then send again.`,
				);
				throw new Error("workspace file upload failed");
			}
			for (const upload of uploaded) {
				if (!upload.response) {
					continue;
				}
				content.push(
					toWorkspaceFileReferencePart({
						path: upload.response.path,
						name: upload.response.name,
						size: upload.response.size,
						mediaType: upload.response.media_type,
						workspaceId: upload.response.workspace_id,
					}),
				);
			}
			// All entries removed mid-upload leaves nothing to say; the
			// idle chat behaves like a plain empty create.
			if (content.length > 0) {
				// Built outside the try block: React Compiler does not
				// support value blocks inside try/catch.
				const firstMessageReq: TypesGen.CreateChatMessageRequest = {
					content,
					mcp_server_ids:
						mcpServerIds && mcpServerIds.length > 0 ? mcpServerIds : undefined,
					plan_mode: planMode === "plan" ? "plan" : undefined,
					...(model ? { model_config_id: model } : {}),
					...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
				};
				const firstMessage = {
					chatId: createdChat.id,
					req: firstMessageReq,
				};
				try {
					await sendFirstMessageMutation.mutateAsync(firstMessage);
				} catch (error) {
					// Without the message the fresh chat is an empty shell,
					// so archive it and stay on the composer with the draft
					// intact; a retry re-creates the chat and re-uploads.
					archiveUnusedChat(createdChat.id);
					toast.error(getErrorMessage(error, "Failed to send the message."));
					throw error;
				}
			}
		}

		navigate({
			pathname: buildAgentChatPath({ chatId: createdChat.id }),
			search: location.search,
		});
	};

	const handleChimeToggle = () => {
		const next = !chimeEnabled;
		setChimeEnabledState(next);
		setChimeEnabled(next);
	};

	const handleNotificationToggle = async () => {
		try {
			if (webPush.subscribed) {
				await webPush.unsubscribe();
			} else {
				await webPush.subscribe();
			}
		} catch (error) {
			const action = webPush.subscribed ? "disable" : "enable";
			toast.error(getErrorMessage(error, `Failed to ${action} notifications.`));
		}
	};

	return (
		<>
			<AgentPageHeader
				chimeEnabled={chimeEnabled}
				onToggleChime={handleChimeToggle}
				webPush={webPush}
				onToggleNotifications={handleNotificationToggle}
			>
				<ChimeButton enabled={chimeEnabled} onToggle={handleChimeToggle} />
				<WebPushButton webPush={webPush} onToggle={handleNotificationToggle} />
			</AgentPageHeader>
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
			/>{" "}
		</>
	);
};

export default AgentCreatePage;
