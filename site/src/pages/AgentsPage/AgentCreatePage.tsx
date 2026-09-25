import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createChat } from "#/api/queries/chats";
import {
	workspaceBuildById,
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
import { debugWorkspaceBuildSearchParam } from "#/modules/workspaces/workspaceBuildDebugLink";
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
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

// The deep link's build ID moves from the URL into this entry's history state
// on arrival, because the layout's links forward location.search and the
// next composer must be a plain one.
type DebugLinkState = { debugWorkspaceBuildId: string };

const readDebugLinkState = (state: unknown): string | null =>
	typeof state === "object" &&
	state !== null &&
	"debugWorkspaceBuildId" in state &&
	typeof state.debugWorkspaceBuildId === "string"
		? state.debugWorkspaceBuildId
		: null;

type DebugWorkspaceBuildAlertProps = {
	error: unknown;
	build: TypesGen.WorkspaceBuild | undefined;
};

const DebugWorkspaceBuildAlert: FC<DebugWorkspaceBuildAlertProps> = ({
	error,
	build,
}) => {
	const alert =
		error != null ? (
			<Alert severity="error" prominent>
				<AlertTitle>Could not load the workspace build or its logs</AlertTitle>
				<AlertDescription>
					{getErrorMessage(error, "The request failed.")}
				</AlertDescription>
			</Alert>
		) : build && build.job.status !== "failed" ? (
			<Alert severity="info">
				<AlertTitle>Nothing to debug</AlertTitle>
				<AlertDescription>
					Build #{build.build_number} of workspace {build.workspace_owner_name}/
					{build.workspace_name} has not failed (status: {build.job.status}).
				</AlertDescription>
			</Alert>
		) : null;
	return alert ? (
		<div className="mx-auto w-full max-w-3xl px-4 pt-4">{alert}</div>
	) : null;
};

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

	const debugLinkParam = searchParams.get(debugWorkspaceBuildSearchParam);
	const debugLinkValue = debugLinkParam ?? readDebugLinkState(location.state);
	const debugBuildId =
		debugLinkValue !== null &&
		isUUID(debugLinkValue) &&
		experiments.includes("enable-ai-workspace-debug")
			? debugLinkValue
			: null;
	useEffect(() => {
		if (debugLinkParam === null) {
			return;
		}
		const search = new URLSearchParams(searchParams);
		search.delete(debugWorkspaceBuildSearchParam);
		const state: DebugLinkState | undefined =
			debugBuildId !== null
				? { debugWorkspaceBuildId: debugBuildId }
				: undefined;
		navigate(
			{ pathname: location.pathname, search: search.toString() },
			{ replace: true, state },
		);
	}, [debugLinkParam, debugBuildId, location.pathname, navigate, searchParams]);
	const debugBuildQuery = useQuery({
		...workspaceBuildById(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
		// A reconnect refetch after a load error would replace the composer the
		// user may already be using with the prefilled form.
		refetchOnReconnect: false,
	});
	const debugBuild = debugBuildQuery.data;
	const debugBuildFailed = debugBuild?.job.status === "failed";
	const debugBuildLogsQuery = useQuery({
		...workspaceBuildLogs(debugBuildId ?? ""),
		// The logs query never refetches, so fetch only once the build has failed.
		enabled: debugBuildFailed,
	});
	const prefillError = debugBuildQuery.error ?? debugBuildLogsQuery.error;
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
				}
			: undefined;
	// Hold the form until the prefill is ready: AgentCreateForm reads message
	// and attachment only on mount.
	const isPrefillLoading =
		debugBuildId !== null &&
		prefillError == null &&
		(debugBuild === undefined ||
			(debugBuildFailed && debugBuildLogsQuery.data === undefined));

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
			<DebugWorkspaceBuildAlert error={prefillError} build={debugBuild} />
			{isPrefillLoading ? (
				<Loader className="flex-1" label="Loading workspace build logs" />
			) : (
				<AgentCreateForm
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
