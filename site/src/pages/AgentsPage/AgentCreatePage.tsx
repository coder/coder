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
import {
	debugWorkspaceBuildSearchParam,
	takeDebugWorkspaceBuildIntent,
} from "#/modules/workspaces/workspaceBuildDebugLink";
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

const lastModelConfigIDStorageKey = "agents.last-model-config-id";

type DebugLink =
	| { kind: "none" }
	| { kind: "experiment-disabled" }
	| { kind: "invalid" }
	| { kind: "build"; buildId: string; clicked: boolean };

const readDebugLink = (
	param: string | null,
	experiments: readonly TypesGen.Experiment[],
): DebugLink => {
	if (param === null) {
		return { kind: "none" };
	}
	if (!experiments.includes("enable-ai-workspace-debug")) {
		return { kind: "experiment-disabled" };
	}
	if (!isUUID(param)) {
		return { kind: "invalid" };
	}
	return {
		kind: "build",
		buildId: param,
		clicked: takeDebugWorkspaceBuildIntent(param),
	};
};

const AgentCreatePage: FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams, setSearchParams] = useSearchParams();
	const { permissions, user } = useAuthenticated();
	const { experiments } = useDashboard();
	const aiGatewayDisabled = !useAIGatewayEnabled();
	const workspacesQuery = useQuery(workspaces({ q: "owner:me", limit: 0 }));
	const createMutation = useMutation(createChat(queryClient));
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	// Consumed once per page load. The intent is taken in the same step so a
	// second tab for the same link cannot also read it.
	const [debugLink] = useState(() =>
		readDebugLink(
			searchParams.get(debugWorkspaceBuildSearchParam),
			experiments,
		),
	);
	// The layout's links forward location.search, so the param must not stay
	// in the URL once it has been read.
	useEffect(() => {
		if (searchParams.has(debugWorkspaceBuildSearchParam)) {
			const next = new URLSearchParams(searchParams);
			next.delete(debugWorkspaceBuildSearchParam);
			setSearchParams(next, { replace: true });
		}
	}, [searchParams, setSearchParams]);
	const debugBuildId = debugLink.kind === "build" ? debugLink.buildId : null;
	const debugBuildQuery = useQuery({
		...workspaceBuild(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
		// A refetch after an error would mount the prefilled form after the page
		// already reported that nothing was sent.
		refetchOnMount: false,
		refetchOnReconnect: false,
		refetchOnWindowFocus: false,
	});
	const debugBuild = debugBuildQuery.data;
	const debugBuildFailed = debugBuild?.job.status === "failed";
	const debugBuildLogsQuery = useQuery({
		...workspaceBuildLogs(debugBuildId ?? ""),
		// The logs query caches forever, which is only right for a finished build.
		enabled: debugBuildFailed,
	});
	const debugBuildError = debugBuildQuery.error ?? debugBuildLogsQuery.error;
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
					// Someone else's build output is only sent after the viewer has
					// seen it and pressed Send.
					autoSend:
						debugLink.kind === "build" &&
						debugLink.clicked &&
						debugBuild.workspace_owner_id === user.id,
				}
			: undefined;
	// AgentCreateForm reads prefill only on mount.
	const isDebugBuildLoading =
		debugBuildId !== null &&
		debugBuildError == null &&
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

		if (model) {
			localStorage.setItem(lastModelConfigIDStorageKey, model);
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

	const debugAlert = (() => {
		if (debugLink.kind === "experiment-disabled") {
			return (
				<Alert severity="info">
					<AlertTitle>This debug link is not enabled here</AlertTitle>
					<AlertDescription>
						Debugging workspace builds with Coder Agents requires the{" "}
						<code>enable-ai-workspace-debug</code> experiment, which is off on
						this deployment. Nothing was sent.
					</AlertDescription>
				</Alert>
			);
		}
		if (debugLink.kind === "invalid") {
			return (
				<Alert severity="info">
					<AlertTitle>This debug link is not valid</AlertTitle>
					<AlertDescription>
						The link does not carry a workspace build ID. Nothing was sent.
					</AlertDescription>
				</Alert>
			);
		}
		if (debugBuildError != null) {
			return (
				<Alert severity="error" prominent>
					<AlertTitle>
						Could not load the failed workspace build. Nothing was sent.
					</AlertTitle>
					<AlertDescription>
						{getErrorMessage(debugBuildError, "The request failed.")}
					</AlertDescription>
				</Alert>
			);
		}
		if (debugBuild && !debugBuildFailed) {
			return (
				<Alert severity="info">
					<AlertTitle>Nothing to debug</AlertTitle>
					<AlertDescription>
						Build #{debugBuild.build_number} of workspace{" "}
						{debugBuild.workspace_owner_name}/{debugBuild.workspace_name} has
						not failed (status: {debugBuild.job.status}).
					</AlertDescription>
				</Alert>
			);
		}
		return null;
	})();

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
			{debugAlert && (
				<div className="mx-auto w-full max-w-3xl px-4 pt-4">{debugAlert}</div>
			)}
			{isDebugBuildLoading ? (
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
