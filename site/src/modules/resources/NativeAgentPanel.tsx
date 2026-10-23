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
	if (user?.id !== workspace.owner_id) return null;
	return (
		<NativeAgentControls key={agent.id} agent={agent} workspace={workspace} />
	);
};

/** Owns the panel's ACP connection; unmounting disconnects ACP, not the native session. */
export const NativeAgentControls: React.FC<NativeAgentPanelProps> = ({
	agent,
	workspace,
}) => {
	const [session] = useState(() => new NativeAgentSession(agent.id));
	const snapshot = useSyncExternalStore(session.subscribe, session.getSnapshot);
	const agentAvailable =
		agent.status === "connected" &&
		["ready", "start_error", "start_timeout"].includes(agent.lifecycle_state);
	const [navigationError, setNavigationError] = useState<string>();
	const [collapsedSessionID, setCollapsedSessionID] = useState<string>();
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
		initialValues: { cwd: agent.expanded_directory || agent.directory || "" },
		onSubmit: ({ cwd }, helpers) => {
			if (operationsBlocked || snapshot.session?.prompting) {
				helpers.setSubmitting(false);
				return;
			}
			setNavigationError(undefined);
			setCollapsedSessionID(undefined);
			if (!sendPrompt.isPending) sendPrompt.reset();
			action.mutate(
				{ type: "create", cwd },
				{
					onSettled: () => helpers.setSubmitting(false),
				},
			);
		},
	});
	const promptForm = useFormik({
		initialValues: { prompt: "" },
		onSubmit: ({ prompt }, helpers) => {
			if (
				operationsBlocked ||
				snapshot.session?.prompting ||
				snapshot.session?.error ||
				nativeAgentMetadata(snapshot.session?.info).unavailable ||
				nativeAgentMetadata(snapshot.session?.info).registrationPending ||
				snapshot.session?.modes?.currentModeId !== "automation" ||
				!prompt.trim()
			) {
				helpers.setSubmitting(false);
				return;
			}
			setNavigationError(undefined);
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
		setNavigationError(undefined);
		if (!sendPrompt.isPending) sendPrompt.reset();
		if (request.type === "join") {
			const selected =
				request.session.sessionId === snapshot.session?.info.sessionId;
			if (
				nativeAgentMetadata(request.session).unavailable ||
				(selected && snapshot.session?.error) ||
				(!selected && snapshot.session?.prompting)
			)
				return;
			setCollapsedSessionID(undefined);
			// Reserve the tab within the user gesture, but navigate only after mode acknowledgement.
			const popup = window.open("about:blank", "_blank");
			if (!popup) {
				setNavigationError("Allow popups to join the native terminal.");
				return;
			}
			popup.opener = null;
			const href = getTerminalHref({
				username: workspace.owner_name,
				workspace: workspace.name,
				agent: agent.name,
				// Keep the terminal alive after detach so reconnect does not rerun attach.
				command: `coder-acp attach '${request.session.sessionId.replaceAll("'", "'\\''")}'; exec "\${SHELL:-/bin/sh}" -l`,
			});
			void action.mutateAsync(request).then(
				() => {
					popup.location.href = href;
				},
				() => popup.close(),
			);
			return;
		}
		action.mutate(request, {
			onSuccess: () => {
				if (request.type === "connect" || request.type === "load")
					sendPrompt.reset();
			},
		});
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
			error={
				navigationError || action.error?.message || sendPrompt.error?.message
			}
			busy={action.isPending}
			hasMoreSessions={sessions.hasNextPage}
			cwd={cwdForm.values.cwd}
			prompt={promptForm.values.prompt}
			onCwdChange={cwdForm.handleChange}
			onPromptChange={promptForm.handleChange}
			onCreateSubmit={cwdForm.handleSubmit}
			onPromptSubmit={promptForm.handleSubmit}
			onAction={onAction}
			onRefresh={() => {
				if (!operationsBlocked) void sessions.refetch();
			}}
			onMoreSessions={() => {
				if (!operationsBlocked) void sessions.fetchNextPage();
			}}
		/>
	);
};
