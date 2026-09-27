import { type FC, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import {
	type ApiErrorResponse,
	getErrorMessage,
	isApiError,
} from "#/api/errors";
import {
	archiveChat,
	createChat,
	createChatMessageByChatId,
	invalidateChatListQueries,
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

const isConflictError = (error: unknown) =>
	isApiError(error) && error.response.status === 409;

// Server-side hook dispatch is capped at 5 seconds, so a create or first
// message still pending after this is stalled rather than slow.
const chatRequestTimeoutMs = 30_000;
const cleanupArchiveTimeoutMs = 10_000;

class RequestTimeoutError extends Error {
	constructor() {
		super("The request timed out.");
		this.name = "RequestTimeoutError";
	}
}

// Rejects with RequestTimeoutError once timeoutMs passes, even when the
// request ignores the signal; the signal cancels requests that accept it.
const withTimeout = async <T,>(
	request: (signal: AbortSignal) => Promise<T>,
	timeoutMs: number,
): Promise<T> => {
	const controller = new AbortController();
	const timedOut = new Promise<never>((_, reject) => {
		controller.signal.addEventListener("abort", () =>
			reject(new RequestTimeoutError()),
		);
	});
	const timer = setTimeout(() => controller.abort(), timeoutMs);
	try {
		return await Promise.race([request(controller.signal), timedOut]);
	} finally {
		clearTimeout(timer);
	}
};

const createTimeoutError: ApiErrorResponse = {
	message: "Creating the chat took too long.",
	detail:
		"The chat may still have been created. Check the chat list before sending again.",
};

const sendTimeoutError: ApiErrorResponse = {
	message: "Sending the message took too long.",
	detail: "The message was not sent. Try again.",
};

const uncertainSendTimeoutError: ApiErrorResponse = {
	message: "Sending the message took too long.",
	detail:
		"The message may still have been sent. Check the chat list before sending again.",
};

const pageLeftMessage = "The page was left before the first message was sent.";

const cleanupFailureMessage = (error: unknown) =>
	error instanceof RequestTimeoutError
		? "Failed to clean up the unused chat."
		: getErrorMessage(error, "Failed to clean up the unused chat.");

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
	const sendFirstMessageMutation = useMutation(
		createChatMessageByChatId(queryClient),
	);
	const archiveMutation = useMutation(archiveChat(queryClient));
	const [submitError, setSubmitError] = useState<unknown>(null);
	// A submit outlives the page when the user leaves mid-request. After
	// that it must not navigate or report anything, but its requests keep
	// running: a first message that commits is a real chat.
	const isMountedRef = useRef(false);
	useEffect(() => {
		isMountedRef.current = true;
		return () => {
			isMountedRef.current = false;
		};
	}, []);
	const reportSubmitError = (error: unknown) => {
		if (isMountedRef.current) {
			setSubmitError(error);
		}
	};
	// Cleanup for a shell chat whose deferred uploads failed or whose
	// page was left before the first message. The caller reports the
	// primary failure, so this only adds the cleanup outcome.
	const archiveUnusedChat = (chatId: string) => {
		archiveMutation.mutate(chatId, {
			onError: (error) => {
				if (isMountedRef.current) {
					toast.error(cleanupFailureMessage(error));
				}
			},
		});
	};
	// A failed send can still commit server-side. Archiving returns 409
	// only while the reply is generating; once it finishes the chat is
	// idle again and would archive, so check for messages first. Only a
	// successful archive proves the message is absent: an archived chat
	// refuses a late send.
	const archiveChatAfterFailedSend = async (
		chatId: string,
	): Promise<"committed" | "archived" | "unknown"> => {
		try {
			const { messages } = await withTimeout(
				() => API.experimental.getChatMessages(chatId, { limit: 1 }),
				cleanupArchiveTimeoutMs,
			);
			if (messages.length > 0) {
				return "committed";
			}
			await withTimeout(
				() => archiveMutation.mutateAsync(chatId),
				cleanupArchiveTimeoutMs,
			);
			return "archived";
		} catch (error) {
			if (isConflictError(error)) {
				return "committed";
			}
			if (isMountedRef.current) {
				toast.error(cleanupFailureMessage(error));
			}
			return "unknown";
		}
	};
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
		setSubmitError(null);
		let createdChat: TypesGen.Chat;
		try {
			createdChat = await withTimeout(
				(signal) => createMutation.mutateAsync({ req: createRequest, signal }),
				chatRequestTimeoutMs,
			);
		} catch (error) {
			if (error instanceof RequestTimeoutError) {
				// The chat may exist without the page knowing its ID, so
				// refresh the sidebar where it would show up.
				void invalidateChatListQueries(queryClient);
				reportSubmitError(createTimeoutError);
			} else {
				reportSubmitError(error);
			}
			throw error;
		}

		if (uploadWorkspaceFiles) {
			if (!isMountedRef.current) {
				archiveUnusedChat(createdChat.id);
				throw new Error(pageLeftMessage);
			}
			let uploaded: Awaited<ReturnType<typeof uploadWorkspaceFiles>>;
			try {
				uploaded = await uploadWorkspaceFiles(createdChat.id);
			} catch (error) {
				// The empty chat never started generating, so archiving it
				// right away is the cleanup path; retry creates a fresh one.
				archiveUnusedChat(createdChat.id);
				if (isMountedRef.current && !isAbortError(error)) {
					toast.error(
						getErrorMessage(error, "Failed to upload files to the workspace."),
					);
				}
				throw error;
			}
			if (!isMountedRef.current) {
				archiveUnusedChat(createdChat.id);
				throw new Error(pageLeftMessage);
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
				let sendFailed = false;
				let sendError: unknown;
				try {
					await withTimeout(
						(signal) =>
							sendFirstMessageMutation.mutateAsync({
								chatId: createdChat.id,
								req: firstMessageReq,
								signal,
							}),
						chatRequestTimeoutMs,
					);
				} catch (error) {
					sendFailed = true;
					sendError = error;
				}
				// Without the message the fresh chat is an empty shell, so
				// archive it and stay on the composer with the draft intact;
				// a retry re-creates the chat and re-uploads.
				if (sendFailed) {
					const cleanup = await archiveChatAfterFailedSend(createdChat.id);
					if (cleanup !== "committed") {
						let reportedError = sendError;
						if (sendError instanceof RequestTimeoutError) {
							reportedError =
								cleanup === "archived"
									? sendTimeoutError
									: uncertainSendTimeoutError;
						}
						reportSubmitError(reportedError);
						throw sendError;
					}
				}
			}
		}

		if (!isMountedRef.current) {
			return;
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
					key={prefill ? debugBuildId : "draft"}
					onCreateChat={handleCreateChat}
					isCreating={createMutation.isPending}
					createError={submitError}
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
