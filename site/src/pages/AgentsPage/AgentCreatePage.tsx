import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createChat } from "#/api/queries/chats";
import {
	workspaceBuild,
	workspaceBuildLogs,
} from "#/api/queries/workspaceBuilds";
import { workspaces } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	type AgentCreateAutoSubmit,
	AgentCreateForm,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import { WebPushButton } from "./components/WebPushButton";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { buildAgentChatPath } from "./utils/navigation";
import {
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	debugWorkspaceBuildSearchParam,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	const debugBuildId = experiments.includes("enable-ai-workspace-debug")
		? searchParams.get(debugWorkspaceBuildSearchParam)
		: null;
	const debugBuildQuery = useQuery({
		...workspaceBuild(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
	});
	const debugBuildLogsQuery = useQuery({
		...workspaceBuildLogs(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
	});
	const debugBuildError = debugBuildQuery.error ?? debugBuildLogsQuery.error;
	const autoSubmit: AgentCreateAutoSubmit | undefined =
		debugBuildQuery.data && debugBuildLogsQuery.data
			? {
					message: debugWorkspaceBuildPrompt(debugBuildQuery.data.transition),
					attachment: {
						name: debugWorkspaceBuildLogsFileName(debugBuildQuery.data),
						content: formatWorkspaceBuildLogsForDebug(
							debugBuildQuery.data,
							debugBuildLogsQuery.data,
						),
					},
				}
			: undefined;
	// The form reads the auto-submitted message as its initial editor value,
	// so it must not mount until the build and logs have loaded.
	const isDebugBuildLoading =
		debugBuildId !== null &&
		autoSubmit === undefined &&
		debugBuildError == null;

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
			content,
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
		// Drop the debug deep link so returning to /agents from the new chat
		// does not start another one.
		const nextSearchParams = new URLSearchParams(location.search);
		nextSearchParams.delete(debugWorkspaceBuildSearchParam);
		navigate({
			pathname: buildAgentChatPath({ chatId: createdChat.id }),
			search: nextSearchParams.toString(),
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
			{debugBuildError != null && (
				<div className="mx-auto w-full max-w-3xl px-4 pt-4">
					<ErrorAlert error={debugBuildError} />
				</div>
			)}
			{isDebugBuildLoading ? (
				<Loader className="flex-1" label="Loading workspace build logs" />
			) : (
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
					autoSubmit={autoSubmit}
				/>
			)}
		</>
	);
};

export default AgentCreatePage;
