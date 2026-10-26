import type { SessionInfo } from "@agentclientprotocol/sdk";
import { useFormik } from "formik";
import { useEffect, useState, useSyncExternalStore } from "react";
import { useInfiniteQuery, useMutation, useQueryClient } from "react-query";
import {
	type NativeAgentAction,
	nativeAgentAction,
	nativeAgentSessions,
} from "#/api/queries/workspaceAgentACP";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { getTerminalHref } from "#/modules/apps/apps";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import {
	nativeAgentPanelStorageKey,
	readNativeAgentPanelState,
	writeNativeAgentPanelState,
} from "./NativeAgentPanelStorage";
import { NativeAgentPanelView } from "./NativeAgentPanelView";
import { NativeAgentSession } from "./NativeAgentSession";
import { nativeAgentMetadata } from "./nativeAgentMetadata";

type NativeAgentPanelProps = { agent: WorkspaceAgent; workspace: Workspace };

/** Discovers native sessions for the owner's experiment-enabled Linux workspace. */
export const NativeAgentPanel: React.FC<NativeAgentPanelProps> = ({
	agent,
	workspace,
}) => {
	const { experiments } = useDashboard();
	const { browser_only } = useFeatureVisibility();
	if (
		!experiments.includes("native-agents") ||
		browser_only ||
		agent.operating_system !== "linux"
	)
		return null;
	return <OwnedNativeAgentPanel agent={agent} workspace={workspace} />;
};

const OwnedNativeAgentPanel: React.FC<NativeAgentPanelProps> = ({
	agent,
	workspace,
}) => {
	const { user } = useAuthContext();
	if (!user || user.id !== workspace.owner_id) return null;
	const storageKey = nativeAgentPanelStorageKey(
		user.id,
		workspace.id,
		agent.name,
	);
	return (
		<NativeAgentControls
			key={JSON.stringify([storageKey, agent.id])}
			agent={agent}
			workspace={workspace}
			storageKey={storageKey}
		/>
	);
};

type NativeAgentControlsProps = NativeAgentPanelProps & { storageKey: string };

/** Owns the panel's ACP connection; unmounting disconnects ACP, not the native session. */
export const NativeAgentControls: React.FC<NativeAgentControlsProps> = ({
	agent,
	workspace,
	storageKey,
}) => {
	const [initialState] = useState(() => readNativeAgentPanelState(storageKey));
	const [session] = useState(
		() => new NativeAgentSession(agent.id, initialState?.selectedSession),
	);
	const snapshot = useSyncExternalStore(session.subscribe, session.getSnapshot);
	const agentAvailable =
		agent.status === "connected" &&
		["ready", "start_error", "start_timeout"].includes(agent.lifecycle_state);
	const [collapsedSessionID, setCollapsedSessionID] = useState(
		initialState?.collapsedSessionID,
	);
	const queryClient = useQueryClient();
	const action = useMutation(nativeAgentAction(session, queryClient));
	const sendPrompt = useMutation(nativeAgentAction(session, queryClient));
	const { mutate: connect } = useMutation(
		nativeAgentAction(session, queryClient),
	);
	const sessions = useInfiniteQuery({
		...nativeAgentSessions(session),
		enabled: agentAvailable && snapshot.status === "connected",
	});
	useEffect(() => {
		// Startup installs the adapter. Each completed startup gets one connection attempt.
		const controller = new AbortController();
		if (agentAvailable) connect({ type: "connect", signal: controller.signal });
		return () => {
			controller.abort();
			session.close();
		};
	}, [agentAvailable, connect, session]);

	const operationsBlocked =
		!agentAvailable || snapshot.status !== "connected" || action.isPending;
	const cwdForm = useFormik({
		initialValues: {
			cwd:
				initialState?.cwd ??
				(agent.expanded_directory || agent.directory || ""),
			harness: initialState?.harness ?? "",
		},
		onSubmit: ({ cwd }, helpers) => {
			if (operationsBlocked || snapshot.session?.prompting) {
				helpers.setSubmitting(false);
				return;
			}
			setCollapsedSessionID(undefined);
			if (!sendPrompt.isPending) sendPrompt.reset();
			action.mutate(
				{ type: "create", cwd, harness },
				{
					onSettled: () => helpers.setSubmitting(false),
				},
			);
		},
	});
	const harness = cwdForm.values.harness || snapshot.harnesses?.[0]?.id;
	const promptForm = useFormik({
		initialValues: { prompt: initialState?.prompt ?? "" },
		onSubmit: ({ prompt }, helpers) => {
			const metadata = nativeAgentMetadata(snapshot.session?.info);
			if (
				operationsBlocked ||
				snapshot.session?.prompting ||
				snapshot.session?.error ||
				metadata.unavailable ||
				metadata.registrationPending ||
				metadata.resumePending ||
				metadata.resumable ||
				snapshot.session?.modes?.currentModeId !== "automation" ||
				!prompt.trim()
			) {
				helpers.setSubmitting(false);
				return;
			}
			action.reset();
			sendPrompt.mutate(
				{ type: "prompt", text: prompt },
				{
					onSuccess: () => {
						void helpers.setValues((current) =>
							current.prompt === prompt ? { prompt: "" } : current,
						);
					},
					onSettled: () => helpers.setSubmitting(false),
				},
			);
		},
	});

	const selectedInfo = snapshot.session?.info ?? initialState?.selectedSession;
	const selectedSessionID = selectedInfo?.sessionId;
	const selectedCwd = selectedInfo?.cwd;
	useEffect(() => {
		// Form edits, send completion, and ACP selection updates share this storage boundary.
		// Read only on mount so background updates cannot replace an in-progress draft.
		writeNativeAgentPanelState(storageKey, {
			cwd: cwdForm.values.cwd,
			harness: harness ?? "",
			prompt: promptForm.values.prompt,
			collapsedSessionID,
			selectedSession:
				selectedSessionID && selectedCwd
					? { sessionId: selectedSessionID, cwd: selectedCwd }
					: undefined,
		});
	}, [
		storageKey,
		cwdForm.values.cwd,
		harness,
		promptForm.values.prompt,
		collapsedSessionID,
		selectedSessionID,
		selectedCwd,
	]);

	const onAction = (request: NativeAgentAction) => {
		if (
			request.type === "load" &&
			request.session.sessionId === snapshot.session?.info.sessionId
		) {
			setCollapsedSessionID((current) =>
				current === request.session.sessionId
					? undefined
					: request.session.sessionId,
			);
			return;
		}
		if (!agentAvailable || (request.type !== "connect" && operationsBlocked))
			return;
		if (request.type === "load") setCollapsedSessionID(undefined);
		if (!sendPrompt.isPending) sendPrompt.reset();
		action.mutate(request, {
			onSuccess: () => {
				if (request.type === "connect" || request.type === "load")
					sendPrompt.reset();
			},
		});
	};
	const onOpen = (info: SessionInfo) => {
		if (!agentAvailable) return;
		window.open(
			getTerminalHref({
				username: workspace.owner_name,
				workspace: workspace.name,
				agent: agent.name,
				// Keep the shell alive after detach or a diagnostic; reconnect must not rerun attach.
				command: `coder-acp attach '${info.sessionId.replaceAll("'", "'\\''")}'; exec "\${SHELL:-/bin/sh}" -l`,
			}),
			"_blank",
			"noopener,noreferrer",
		);
	};
	const listedSessions =
		sessions.data?.pages.flatMap((page) => page.sessions) ?? [];
	const active = snapshot.session?.info;
	const sessionOptions = listedSessions.map((item) =>
		item.sessionId === active?.sessionId ? active : item,
	);
	if (
		active &&
		!sessionOptions.some((item) => item.sessionId === active.sessionId)
	)
		sessionOptions.unshift(active);
	return (
		<NativeAgentPanelView
			agentStatus={agent.status}
			agentReady={agentAvailable}
			snapshot={snapshot}
			sessionExpanded={snapshot.session?.info.sessionId !== collapsedSessionID}
			sessions={sessionOptions}
			loadingSessions={sessions.isLoading}
			refreshingSessions={sessions.isFetching}
			sessionsError={sessions.error?.message}
			error={action.error?.message || sendPrompt.error?.message}
			busy={action.isPending}
			hasMoreSessions={sessions.hasNextPage}
			cwd={cwdForm.values.cwd}
			harness={harness}
			prompt={promptForm.values.prompt}
			onCwdChange={cwdForm.handleChange}
			onHarnessChange={(value) => void cwdForm.setFieldValue("harness", value)}
			onPromptChange={promptForm.handleChange}
			onCreateSubmit={cwdForm.handleSubmit}
			onPromptSubmit={promptForm.handleSubmit}
			onAction={onAction}
			onOpen={onOpen}
			onRefresh={() => {
				if (!operationsBlocked) void sessions.refetch();
			}}
			onMoreSessions={() => {
				if (!operationsBlocked) void sessions.fetchNextPage();
			}}
		/>
	);
};
