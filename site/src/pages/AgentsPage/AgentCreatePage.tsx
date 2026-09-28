import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage, isApiError } from "#/api/errors";
import {
	archiveChat,
	createChat,
	createChatMessageByChatId,
} from "#/api/queries/chats";
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
import { isAbortError } from "./utils/chatAttachments";
import { toWorkspaceFileReferencePart } from "./utils/chatInputContent";
import { getChimeEnabled, setChimeEnabled } from "./utils/chime";
import { type DeepLinkState, readDeepLinkState } from "./utils/deepLinkState";
import { buildAgentChatPath } from "./utils/navigation";
import {
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

const promptSearchParam = "prompt";

type DebugWorkspaceBuildAlertProps = {
	error: unknown;
	build: TypesGen.WorkspaceBuild | undefined;
};

const DebugWorkspaceBuildAlert: React.FC<DebugWorkspaceBuildAlertProps> = ({
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

const isConflictError = (error: unknown) =>
	isApiError(error) && error.response.status === 409;

const AgentCreatePage: React.FC = () => {
	const queryClient = useQueryClient();
	const location = useLocation();
	const navigate = useNavigate();
	const [searchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
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
	// Resolves true when the chat already holds the first message. A
	// failed send can still commit server-side; the chat then left the
	// idle state and archiving it returns 409.
	const archiveChatAfterFailedSend = async (chatId: string) => {
		try {
			await archiveMutation.mutateAsync(chatId);
		} catch (error) {
			if (isConflictError(error)) {
				return true;
			}
			toast.error(
				getErrorMessage(error, "Failed to clean up the unused chat."),
			);
		}
		return false;
	};
	const webPush = useWebpushNotifications();
	const [chimeEnabled, setChimeEnabledState] = useState(getChimeEnabled);

	const linkState = readDeepLinkState(location.state);
	const debugLinkParam = searchParams.get(debugWorkspaceBuildSearchParam);
	const debugLinkValue = debugLinkParam ?? linkState.debugWorkspaceBuildId;
	const debugBuildId =
		debugLinkValue !== undefined &&
		isUUID(debugLinkValue) &&
		experiments.includes("enable-ai-workspace-debug")
			? debugLinkValue
			: null;
	const promptParam = searchParams.get(promptSearchParam);
	const promptValue = promptParam ?? linkState.prompt;
	// The debug link wins when both are present.
	const linkPrompt =
		debugBuildId === null && promptValue?.trim() ? promptValue : undefined;
	// One navigation removes both parameters: separate navigations would each
	// rebuild the URL from the same snapshot and restore the other parameter.
	useEffect(() => {
		if (debugLinkParam === null && promptParam === null) {
			return;
		}
		const search = new URLSearchParams(searchParams);
		search.delete(debugWorkspaceBuildSearchParam);
		search.delete(promptSearchParam);
		const state: DeepLinkState | undefined =
			debugBuildId !== null
				? { debugWorkspaceBuildId: debugBuildId }
				: linkPrompt !== undefined
					? { prompt: linkPrompt }
					: undefined;
		navigate(
			{ pathname: location.pathname, search: search.toString() },
			{ replace: true, state },
		);
	}, [
		debugLinkParam,
		promptParam,
		debugBuildId,
		linkPrompt,
		location.pathname,
		navigate,
		searchParams,
	]);
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
	const debugPrefill: AgentCreatePrefill | undefined =
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
	const prefill: AgentCreatePrefill | undefined =
		debugPrefill ??
		(linkPrompt
			? {
					message: linkPrompt,
					warning:
						"Use caution before running this prompt. Malicious content could trick Coder Agents into attempting harmful actions or sharing your data.",
				}
			: undefined);
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

		if (uploadWorkspaceFiles) {
			let uploaded: Awaited<ReturnType<typeof uploadWorkspaceFiles>>;
			try {
				uploaded = await uploadWorkspaceFiles(createdChat.id);
			} catch (error) {
				// The empty chat never started generating, so archiving it
				// right away is the cleanup path; retry creates a fresh one.
				archiveUnusedChat(createdChat.id);
				if (!isAbortError(error)) {
					toast.error(
						getErrorMessage(error, "Failed to upload files to the workspace."),
					);
				}
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
				let sendFailed = false;
				let sendError: unknown;
				try {
					await sendFirstMessageMutation.mutateAsync(firstMessage);
				} catch (error) {
					sendFailed = true;
					sendError = error;
				}
				// Without the message the fresh chat is an empty shell, so
				// archive it and stay on the composer with the draft intact;
				// a retry re-creates the chat and re-uploads.
				if (sendFailed && !(await archiveChatAfterFailedSend(createdChat.id))) {
					toast.error(
						getErrorMessage(sendError, "Failed to send the message."),
					);
					throw sendError;
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
			<DebugWorkspaceBuildAlert error={prefillError} build={debugBuild} />
			{isPrefillLoading ? (
				<Loader className="flex-1" label="Loading workspace build logs" />
			) : (
				<AgentCreateForm
					key={
						debugPrefill
							? debugBuildId
							: linkPrompt
								? `prompt:${linkPrompt}`
								: "draft"
					}
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
