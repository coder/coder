import type { SessionInfo } from "@agentclientprotocol/sdk";
import { EyeIcon, PlusIcon, RefreshCwIcon, TerminalIcon } from "lucide-react";
import { useId } from "react";
import type { NativeAgentAction } from "#/api/queries/workspaceAgentACP";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import type { NativeAgentSnapshot } from "./NativeAgentSession";
import { NativeAgentSessionCard } from "./NativeAgentSessionCard";
import {
	NativeAgentSessionError,
	NativeAgentSessionStatus,
	NativeAgentTimestamp,
} from "./NativeAgentSessionStatus";
import { NativeAgentTranscript } from "./NativeAgentTranscript";
import { nativeAgentMetadata } from "./nativeAgentMetadata";

type NativeAgentPanelViewProps = {
	agentStatus: WorkspaceAgent["status"];
	agentReady: boolean;
	snapshot: NativeAgentSnapshot;
	sessionExpanded: boolean;
	sessions: readonly SessionInfo[];
	loadingSessions: boolean;
	refreshingSessions: boolean;
	sessionsError?: string;
	error?: string;
	busy: boolean;
	hasMoreSessions: boolean;
	cwd: string;
	prompt: string;
	/** Fixed reference clock for deterministic visual states. */
	now?: number;
	onCwdChange: React.ChangeEventHandler<HTMLInputElement>;
	onPromptChange: React.ChangeEventHandler<HTMLTextAreaElement>;
	onCreateSubmit: React.FormEventHandler<HTMLFormElement>;
	onPromptSubmit: React.FormEventHandler<HTMLFormElement>;
	onAction: (action: NativeAgentAction) => void;
	onRefresh: () => void;
	onMoreSessions: () => void;
};

/** Workspace-native controls independent of the ACP connection lifecycle. */
export const NativeAgentPanelView: React.FC<NativeAgentPanelViewProps> = ({
	agentStatus,
	agentReady,
	snapshot,
	sessionExpanded,
	sessions,
	loadingSessions,
	refreshingSessions,
	sessionsError,
	error,
	busy,
	hasMoreSessions,
	cwd,
	prompt,
	now,
	onCwdChange,
	onPromptChange,
	onCreateSubmit,
	onPromptSubmit,
	onAction,
	onRefresh,
	onMoreSessions,
}) => {
	const id = useId();
	const agentAvailable = agentStatus === "connected";
	const connected = agentReady && snapshot.status === "connected";
	const session = snapshot.session;
	const blocked = !connected || busy;
	const metadata = nativeAgentMetadata(session?.info);
	const sessionUnavailable = Boolean(session?.error) || metadata.unavailable;
	const registrationPending = metadata.registrationPending;
	const observe =
		session?.modes?.currentModeId === "human" && !registrationPending;
	const mode = session?.modes?.currentModeId;
	const agentName = snapshot.agentInfo?.name.trim() ?? "";
	const agentLabel =
		snapshot.agentInfo?.title?.trim() || agentName || "Native agent";
	const canSend =
		!blocked &&
		!session?.prompting &&
		!sessionUnavailable &&
		!registrationPending &&
		mode === "automation";
	let transcriptStatus =
		"Observing live. Join takes human control and opens the native terminal.";
	if (busy) {
		transcriptStatus = "Updating session…";
	} else if (!connected) {
		transcriptStatus = "Transcript paused. Reconnect to resume.";
	} else if (sessionUnavailable) {
		transcriptStatus =
			"Session unavailable. Showing retained details, not a live transcript.";
	} else if (registrationPending) {
		transcriptStatus = `${metadata.blockedReason || "Waiting for native setup."} Join to continue setup. Once setup is complete, select Observe to enable prompts.`;
	} else if (mode === "human") {
		transcriptStatus =
			"You have control in the native terminal. Observe returns control to the agent; it does not close or detach the terminal. Use Ctrl+Z in the terminal to detach with coder-acp.";
	} else if (!session?.modes) {
		transcriptStatus =
			"Observing retained events. Control modes are unavailable.";
	}
	let promptStatus =
		"Text prompts only. Permissions stay in the native terminal.";
	if (sessionUnavailable) {
		promptStatus = "Session unavailable. Refresh its status before sending.";
	} else if (registrationPending) {
		promptStatus =
			"Setup pending: Join the native terminal before sending. Resubmit blocked tasks manually after setup.";
	} else if (session?.prompting) {
		promptStatus = "Agent is working…";
	} else if (mode === "human") {
		promptStatus = "Human control: select Observe to send.";
	}
	return (
		<section
			aria-label="Native sessions"
			className="flex min-w-0 flex-col gap-4 text-sm"
		>
			<div className="flex flex-wrap items-center justify-between gap-3">
				<div>
					<h3 className="m-0 text-base font-semibold">Native sessions</h3>
					<p className="mb-0 mt-1 text-content-secondary">
						Select a session to follow its live transcript.
					</p>
				</div>
				{(!connected || session?.error || error) &&
					snapshot.status !== "connecting" &&
					(agentReady || !agentAvailable) && (
						<Button
							variant="outline"
							disabled={!agentReady || busy}
							onClick={() => onAction({ type: "connect" })}
						>
							<RefreshCwIcon />
							Reconnect
						</Button>
					)}
				{snapshot.status === "connecting" && (
					<span
						role="status"
						className="flex items-center gap-2 text-content-secondary"
					>
						<Spinner loading />
						Discovering native sessions…
					</span>
				)}
			</div>
			{agentAvailable && !agentReady && (
				<p role="status" className="m-0 text-content-secondary">
					Waiting for workspace startup to finish before discovering native
					sessions…
				</p>
			)}
			{!agentAvailable && (
				<Alert severity="info">
					<AlertDescription>
						Workspace agent status: {agentStatus}. Your draft, directory, and
						selected session are kept. Discovery resumes when the workspace
						agent connects.
					</AlertDescription>
				</Alert>
			)}
			{(snapshot.error || error) && (
				<ErrorAlert error={snapshot.error || error} prominent={false} />
			)}
			{snapshot.error && !connected && (
				<div className="text-content-secondary">
					Install and configure a native ACP adapter in this workspace.{" "}
					<code>coder-acp acp</code> must be on the SSH command PATH and serve
					ACP over stdio without a PTY. <code>coder-acp attach SESSION_ID</code>{" "}
					must join the same persistent session. Complete provider sign-in in
					the workspace terminal.
				</div>
			)}
			{(connected || session) && (
				<>
					<TooltipProvider>
						<div
							className="flex flex-wrap items-start gap-2"
							role="group"
							aria-label="Available native sessions"
						>
							{sessions.map((item) => (
								<NativeAgentSessionCard
									key={item.sessionId}
									info={item}
									now={now}
									agentInfo={snapshot.agentInfo}
									selected={item.sessionId === session?.info.sessionId}
									expanded={
										sessionExpanded &&
										item.sessionId === session?.info.sessionId
									}
									transcriptID={
										session && sessionExpanded ? `${id}-transcript` : undefined
									}
									disabled={
										item.sessionId !== session?.info.sessionId &&
										(blocked || Boolean(session?.prompting))
									}
									joinDisabled={
										blocked ||
										(item.sessionId === session?.info.sessionId
											? sessionUnavailable
											: Boolean(session?.prompting))
									}
									onSelect={(info) => onAction({ type: "load", session: info })}
									onJoin={(info) => onAction({ type: "join", session: info })}
								/>
							))}
						</div>
					</TooltipProvider>
					<div className="flex flex-wrap items-center gap-2">
						<Button
							variant="subtle"
							size="sm"
							disabled={!connected || refreshingSessions || busy}
							onClick={onRefresh}
						>
							<Spinner loading={refreshingSessions}>
								<RefreshCwIcon />
							</Spinner>
							Refresh sessions
						</Button>
						{hasMoreSessions && (
							<Button
								variant="outline"
								disabled={!connected || refreshingSessions || busy}
								onClick={onMoreSessions}
							>
								More sessions
							</Button>
						)}
					</div>
					{sessionsError && (
						<ErrorAlert
							error={`${sessionsError} Refresh sessions to try again.`}
							prominent={false}
						/>
					)}
					{loadingSessions && sessions.length === 0 && (
						<p role="status" className="m-0 text-content-secondary">
							Loading sessions…
						</p>
					)}
					{!loadingSessions && !sessionsError && sessions.length === 0 && (
						<p className="m-0 text-content-secondary">
							No native sessions yet. Sessions started in this workspace appear
							automatically, or you can add one.
						</p>
					)}
					<Collapsible>
						<CollapsibleTrigger asChild>
							<Button variant="subtle" size="sm">
								<PlusIcon />
								Add session
							</Button>
						</CollapsibleTrigger>
						<CollapsibleContent>
							<form
								className="mt-3 flex flex-wrap items-end gap-2"
								onSubmit={onCreateSubmit}
							>
								<div className="min-w-0 flex-1 space-y-2">
									<Label htmlFor={`${id}-cwd`}>Working directory</Label>
									<Input
										id={`${id}-cwd`}
										name="cwd"
										value={cwd}
										onChange={onCwdChange}
										placeholder="/home/coder/project"
										required
										pattern="/.*"
										title="An absolute workspace directory starting with /"
									/>
								</div>
								<Button
									type="submit"
									variant="outline"
									disabled={
										blocked || session?.prompting || !cwd.startsWith("/")
									}
								>
									Create session
								</Button>
							</form>
						</CollapsibleContent>
					</Collapsible>
				</>
			)}
			{session && sessionExpanded && (
				<div
					id={`${id}-transcript`}
					className="flex min-w-0 flex-col gap-4 rounded-lg border border-solid border-border bg-surface-primary p-4"
				>
					<div className="flex flex-wrap items-start justify-between gap-3">
						<div className="min-w-0 space-y-1">
							<h4 className="m-0 break-words text-base font-semibold">
								{session.info.title || agentLabel}
							</h4>
							<p className="m-0 break-all text-content-secondary">
								Session: <code>{session.info.sessionId}</code>
							</p>
							<p className="m-0 break-all text-content-secondary">
								Directory: <code>{session.info.cwd}</code>
							</p>
							<p className="m-0 text-content-secondary">
								<NativeAgentTimestamp
									timestamp={nativeAgentMetadata(session.info).startedAt}
									prefix="Started"
									now={now}
								/>
							</p>
							<NativeAgentSessionStatus info={session.info} />
						</div>
						<Button
							disabled={
								blocked ||
								sessionUnavailable ||
								!session.modes?.availableModes.some(
									(item) => item.id === (observe ? "automation" : "human"),
								)
							}
							onClick={() =>
								onAction(
									observe
										? { type: "mode", mode: "automation" }
										: { type: "join", session: session.info },
								)
							}
						>
							<Spinner loading={busy}>
								{observe ? <EyeIcon /> : <TerminalIcon />}
							</Spinner>
							{observe ? "Observe" : "Join"}
						</Button>
					</div>
					<p role="status" className="m-0 text-content-secondary">
						{transcriptStatus}
					</p>
					<NativeAgentSessionError info={session.info} now={now} />
					{session.error && (
						<ErrorAlert
							error={`${session.error} Reconnect before continuing.`}
							prominent={false}
						/>
					)}
					{session.needsInput && !registrationPending && (
						<Alert severity="warning">
							<AlertDescription>
								The native agent needs input. Join the terminal to handle its
								native dialog. Permissions cannot be approved remotely.
							</AlertDescription>
						</Alert>
					)}
					{!session.modes && !session.error && (
						<p className="m-0 text-content-secondary">
							This adapter has not provided control modes. Joining and prompting
							are unavailable.
						</p>
					)}
					{session.truncated && (
						<p className="m-0 text-content-warning">
							Older display content was trimmed. Join the native terminal for
							more context.
						</p>
					)}
					<NativeAgentTranscript entries={session.entries} />
					<p className="m-0 text-xs text-content-secondary">
						Only retained session events are shown, not full native history.
						Leaving this page does not stop the native session. Prompts are
						never automatically resent.
					</p>
					<form className="flex flex-col gap-2" onSubmit={onPromptSubmit}>
						<Label htmlFor={`${id}-prompt`}>Prompt</Label>
						<Textarea
							id={`${id}-prompt`}
							name="prompt"
							className="font-mono"
							value={prompt}
							maxLength={32_000}
							onChange={onPromptChange}
							placeholder="What should the agent work on?"
						/>
						<div className="flex flex-wrap items-center gap-2">
							<Button type="submit" disabled={!canSend || !prompt.trim()}>
								Send
							</Button>
							<Button
								variant="outline"
								disabled={!connected || !session.prompting || busy}
								onClick={() => onAction({ type: "cancel" })}
							>
								Cancel prompt
							</Button>
							<span role="status" className="text-content-secondary">
								{promptStatus}
							</span>
						</div>
					</form>
				</div>
			)}
		</section>
	);
};
