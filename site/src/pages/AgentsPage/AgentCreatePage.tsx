import { type FC, useEffect, useState } from "react";
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
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Loader } from "#/components/Loader/Loader";
import { useWebpushNotifications } from "#/contexts/useWebpushNotifications";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAIGatewayEnabled } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { isUUID } from "#/utils/uuid";
import {
	AgentCreateForm,
	type AgentCreatePrefill,
	type CreateChatOptions,
} from "./components/AgentCreateForm";
import { AgentPageHeader } from "./components/AgentPageHeader";
import { ChimeButton } from "./components/ChimeButton";
import { WebPushButton } from "./components/WebPushButton";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { buildAgentChatPath } from "./utils/navigation";
import {
	clearDebugWorkspaceBuildIntent,
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	debugWorkspaceBuildSearchParam,
	formatWorkspaceBuildLogsForDebug,
	hasDebugWorkspaceBuildIntent,
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

	const debugBuildParam = searchParams.get(debugWorkspaceBuildSearchParam);
	const debugBuildId =
		experiments.includes("enable-ai-workspace-debug") &&
		debugBuildParam !== null &&
		isUUID(debugBuildParam)
			? debugBuildParam
			: null;
	// Read once per page load and cleared on mount, so a replayed or shared
	// link can only prefill the chat, never send it.
	const [debugAutoSend] = useState(
		() => debugBuildId !== null && hasDebugWorkspaceBuildIntent(debugBuildId),
	);
	useEffect(() => {
		if (debugBuildId !== null) {
			clearDebugWorkspaceBuildIntent();
		}
	}, [debugBuildId]);
	const debugBuildQuery = useQuery({
		...workspaceBuild(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
		// A finished build is immutable, and a refetch after an error would
		// mount the prefilled form a second time.
		staleTime: Number.POSITIVE_INFINITY,
		refetchOnMount: false,
		refetchOnReconnect: false,
		refetchOnWindowFocus: false,
	});
	const debugBuildLogsQuery = useQuery({
		...workspaceBuildLogs(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
	});
	const debugBuildError = debugBuildQuery.error ?? debugBuildLogsQuery.error;
	const debugBuild = debugBuildQuery.data;
	const debugBuildFailed = debugBuild?.job.status === "failed";
	const prefill: AgentCreatePrefill | undefined =
		debugBuild && debugBuildFailed && debugBuildLogsQuery.data
			? {
					message: debugWorkspaceBuildPrompt(debugBuild),
					attachment: {
						name: debugWorkspaceBuildLogsFileName(debugBuild),
						text: formatWorkspaceBuildLogsForDebug(
							debugBuild,
							debugBuildLogsQuery.data,
						),
					},
					organizationId: debugBuild.job.organization_id,
					autoSend: debugAutoSend,
				}
			: undefined;
	// The form reads prefill only on mount (initial editor value, draft and
	// attachment persistence), so it waits for the build and logs.
	const isDebugBuildLoading =
		debugBuildId !== null &&
		debugBuildError == null &&
		(debugBuild === undefined || debugBuildLogsQuery.data === undefined);

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
		const nextSearchParams = new URLSearchParams(location.search);
		nextSearchParams.delete(debugWorkspaceBuildSearchParam);
		navigate(
			{
				pathname: buildAgentChatPath({ chatId: createdChat.id }),
				search: nextSearchParams.toString(),
			},
			// Back from a prefilled chat should not land on the deep link again.
			{ replace: debugBuildId !== null },
		);
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
					<Alert severity="error" prominent>
						<AlertTitle>
							Could not load the failed workspace build. Nothing was sent.
						</AlertTitle>
						<AlertDescription>
							{getErrorMessage(debugBuildError, "The request failed.")}
						</AlertDescription>
					</Alert>
				</div>
			)}
			{debugBuild && !debugBuildFailed && (
				<div className="mx-auto w-full max-w-3xl px-4 pt-4">
					<Alert severity="info">
						<AlertTitle>Nothing to debug</AlertTitle>
						<AlertDescription>
							Build #{debugBuild.build_number} of workspace{" "}
							{debugBuild.workspace_owner_name}/{debugBuild.workspace_name} did
							not fail.
						</AlertDescription>
					</Alert>
				</div>
			)}
			{isDebugBuildLoading ? (
				<Loader className="flex-1" label="Loading workspace build logs" />
			) : (
				<AgentCreateForm
					// Prefill is read on mount, so switching modes must remount.
					key={prefill ? debugBuildId : "draft"}
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
					prefill={prefill}
				/>
			)}
		</>
	);
};

export default AgentCreatePage;
