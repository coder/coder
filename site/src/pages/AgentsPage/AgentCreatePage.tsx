import { type FC, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { createChat } from "#/api/queries/chats";
import {
	workspaceBuildById,
	workspaceBuildLogs,
	workspaceBuildLogsGcTime,
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

// The deep link's build ID moves from the query string into history state
// after the first render, so it survives reload and Back but not the layout's
// links, which forward location.search to a fresh entry.
type DebugLinkState = { debugWorkspaceBuild: string };

const readDebugLinkState = (state: unknown): string | null =>
	typeof state === "object" &&
	state !== null &&
	"debugWorkspaceBuild" in state &&
	typeof state.debugWorkspaceBuild === "string"
		? state.debugWorkspaceBuild
		: null;

type DebugLink =
	| { kind: "none" }
	| { kind: "experiment-disabled" }
	| { kind: "invalid" }
	| { kind: "build"; buildId: string };

const readDebugLink = (
	value: string | null,
	experiments: readonly TypesGen.Experiment[],
): DebugLink => {
	if (value === null) {
		return { kind: "none" };
	}
	if (!experiments.includes("enable-ai-workspace-debug")) {
		return { kind: "experiment-disabled" };
	}
	if (!isUUID(value)) {
		return { kind: "invalid" };
	}
	return { kind: "build", buildId: value };
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
	const debugLink = readDebugLink(debugLinkValue, experiments);
	const debugBuildId = debugLink.kind === "build" ? debugLink.buildId : null;
	useEffect(() => {
		if (debugLinkParam === null) {
			return;
		}
		const search = new URLSearchParams(searchParams);
		search.delete(debugWorkspaceBuildSearchParam);
		const state: DebugLinkState = { debugWorkspaceBuild: debugLinkParam };
		navigate(
			{ pathname: location.pathname, search: search.toString() },
			{ replace: true, state },
		);
	}, [debugLinkParam, location.pathname, navigate, searchParams]);
	// Taken after commit, not during render, so a render React discards cannot
	// consume the click. The ref keeps StrictMode's second effect run from
	// taking (and losing) it again, and leaving the prefill disarms it so Back
	// cannot send a second time.
	const [debugClicked, setDebugClicked] = useState(false);
	const debugIntentTakenRef = useRef(false);
	useEffect(() => {
		if (debugBuildId === null) {
			setDebugClicked(false);
			return;
		}
		if (debugIntentTakenRef.current) {
			return;
		}
		debugIntentTakenRef.current = true;
		setDebugClicked(takeDebugWorkspaceBuildIntent(debugBuildId));
	}, [debugBuildId]);
	const debugBuildQuery = useQuery({
		...workspaceBuildById(debugBuildId ?? ""),
		enabled: debugBuildId !== null,
		// A failed build does not change, and a refetch after an error would
		// mount the prefilled form after the page already reported that nothing
		// was sent. Cached as long as its logs, which the prefill also needs.
		staleTime: (query) =>
			query.state.data?.job.status === "failed" ? Number.POSITIVE_INFINITY : 0,
		gcTime: workspaceBuildLogsGcTime,
		refetchOnReconnect: false,
	});
	const debugBuild = debugBuildQuery.data;
	const debugBuildFailed = debugBuild?.job.status === "failed";
	const debugBuildLogsQuery = useQuery({
		...workspaceBuildLogs(debugBuildId ?? ""),
		// The logs query never refetches, so fetch only once the build has failed.
		enabled: debugBuildFailed,
	});
	// A build that has not failed is refetched on Back; if that fails, the last
	// status is still shown and the failure is not a load error.
	const debugBuildError =
		(debugBuild === undefined ? debugBuildQuery.error : null) ??
		debugBuildLogsQuery.error;
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
					autoSend: debugClicked,
				}
			: undefined;
	// Hold the form until the prefill is ready: AgentCreateForm reads message
	// and attachment only on mount.
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

		// The strip above may not have committed yet when an automatic send runs.
		const search = new URLSearchParams(location.search);
		search.delete(debugWorkspaceBuildSearchParam);
		navigate({
			pathname: buildAgentChatPath({ chatId: createdChat.id }),
			search: search.toString(),
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
						The workspace build ID in this link is not valid. Open the failed
						workspace and click Debug with Coder Agents again. Nothing was sent.
					</AlertDescription>
				</Alert>
			);
		}
		if (debugBuildError != null) {
			return (
				<Alert severity="error" prominent>
					<AlertTitle>
						Could not load the workspace build or its logs. Nothing was sent.
					</AlertTitle>
					<AlertDescription>
						<span className="block">
							{getErrorMessage(debugBuildError, "The request failed.")}
						</span>
						<span className="mt-1 block">Reload the page to try again.</span>
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
						not failed (status: {debugBuild.job.status}). Nothing was sent.
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
