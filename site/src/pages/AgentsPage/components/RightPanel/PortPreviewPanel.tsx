import {
	ExternalLinkIcon,
	MessageSquarePlusIcon,
	NetworkIcon,
} from "lucide-react";
import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { formatAnnotations } from "#/annotator/formatAnnotations";
import {
	type AnnotationSubmission,
	annotatorQueryParam,
	type HighlightItem,
} from "#/annotator/protocol";
import { getErrorMessage } from "#/api/errors";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { WorkspaceIframe } from "#/modules/apps/WorkspaceAppFrame";
import { portForwardURL } from "#/utils/portForward";
import {
	type ComposerHandle,
	type ComposerSendResult,
	useComposer,
} from "../../context/ComposerContext";
import { useAnnotatorBridge } from "../../hooks/useAnnotatorBridge";
import {
	isTabPopoutMessage,
	postToTabPopout,
	type TabPopoutMessage,
	tabPopoutChannelName,
	tabPopoutPath,
} from "../../utils/rightPanelTabPopout";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";

const sendRetries = 6;
const sendRetryMs = 500;
// Comments accepted from the preview wait to be sent one at a time, so
// they arrive in order. The wait is short: a person makes a comment every
// few seconds, while a page forging submissions could otherwise line up
// chat turns that keep arriving long after annotate mode is off.
const maxPendingSends = 3;
const overlayUnavailableReason =
	"The annotation overlay could not load in this app. It may block external scripts or not serve HTML.";
// The previewed app keeps its own origin and may open windows and
// dialogs, but it cannot navigate the window it is shown in: a page must
// never be able to swap the dashboard for a lookalike.
const previewSandbox =
	"allow-scripts allow-same-origin allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-downloads";
// A sent annotation whose turn never starts within this window is
// forgotten rather than left to light up during an unrelated later turn.
const turnStartGraceMs = 15_000;

// Sends one annotation message, retrying briefly while another submission
// is in flight so a quick second comment is delayed rather than dropped.
// Failures are reported to the user here; the result says whether the
// message was accepted. Kept outside the component, with the queue below,
// because the React Compiler does not yet handle loops in try/catch or
// try/finally.
async function deliver(
	composerRef: React.RefObject<ComposerHandle | undefined>,
	message: string,
): Promise<boolean> {
	try {
		for (let attempt = 0; attempt < sendRetries; attempt++) {
			if ((await composerRef.current?.send(message)) === "sent") {
				return true;
			}
			await new Promise((resolve) => setTimeout(resolve, sendRetryMs));
		}
		toast.error("The chat is busy; the UI annotation was not sent.");
	} catch (error) {
		toast.error(getErrorMessage(error, "Failed to send UI annotation."));
	}
	return false;
}

type SendQueue = {
	chain: Promise<void>;
	// Accepted and not yet finished, including the one being sent.
	pending: number;
	sending: boolean;
	// Sends belong to the authorization they were accepted under. Bumped
	// when it is withdrawn, so sends from before then do not start.
	epoch: number;
};

function newSendQueue(): SendQueue {
	return { chain: Promise.resolve(), pending: 0, sending: false, epoch: 0 };
}

// Queues one message behind the others, calling `onSent` once the chat
// accepts it. Returns false when the queue is full and the message was not
// accepted.
function enqueueSend(
	queue: SendQueue,
	composerRef: React.RefObject<ComposerHandle | undefined>,
	message: string,
	onSent: () => void,
): boolean {
	if (queue.pending >= maxPendingSends) {
		return false;
	}
	const epoch = queue.epoch;
	queue.pending++;
	queue.chain = queue.chain.then(async () => {
		try {
			if (epoch !== queue.epoch) {
				return;
			}
			queue.sending = true;
			if (await deliver(composerRef, message)) {
				onSent();
			}
		} finally {
			queue.sending = false;
			queue.pending--;
		}
	});
	return true;
}

// Drops the sends that have not started and returns how many there were.
// One already in flight completes: its comment was accepted while the
// page was authorized.
function dropQueuedSends(queue: SendQueue): number {
	queue.epoch++;
	return queue.pending - (queue.sending ? 1 : 0);
}

export const PortPreviewPanel: React.FC<{
	chatId: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	tab: Extract<UserRightPanelTab, { kind: "port" }>;
	// Shows the annotate control. Requires the chat-ui-annotations
	// experiment so the app proxy injects the overlay.
	canAnnotate?: boolean;
	// Drives the shimmer over annotated elements: on while the agent works
	// on a sent annotation message, cleared when it stops.
	isAgentWorking?: boolean;
	// Rendered by the tab's own window rather than the chat page. The
	// window is opened to annotate in, so annotate mode starts on, and the
	// control that opens the window becomes a plain link to the app.
	isPopoutWindow?: boolean;
	annotatorReadyTimeoutMs?: number;
}> = ({
	chatId,
	workspace,
	agent,
	host,
	tab,
	canAnnotate = false,
	isAgentWorking = false,
	isPopoutWindow = false,
	annotatorReadyTimeoutMs,
}) => {
	const url = portForwardURL(
		host,
		tab.port,
		agent.name,
		workspace.name,
		workspace.owner_name,
		tab.protocol,
	);
	const unavailableMessage = getUnavailableMessage({ host, agent, url });
	const frameRef = useRef<HTMLIFrameElement>(null);
	const composer = useComposer();
	// Queued retries must use the current committed send gate, not the
	// composer captured when the annotation was accepted.
	const composerRef = useRef(composer);
	useLayoutEffect(() => {
		composerRef.current = composer;
		return () => {
			composerRef.current = undefined;
		};
	}, [composer]);
	const unavailableReasonId = useId();
	// The proxy injects the overlay only on the request that carries the
	// marker param, so each request reloads the frame. Client-side routing
	// inside the app keeps the overlay; a full navigation drops it until
	// the user presses Annotate again. The tab's own window is opened to
	// annotate in, so it asks for the overlay from the start.
	const [overlayRequests, setOverlayRequests] = useState(
		isPopoutWindow ? 1 : 0,
	);
	// Whether a window of the tab's own is showing it. While one is, the
	// panel steps aside rather than run a second preview of the same app,
	// and relays the window's sends through this chat's composer.
	const [hasPopout, setHasPopout] = useState(false);
	// Elements from annotations sent this turn, highlighted in the preview
	// until the agent's turn ends. `started` guards against a send resolving
	// before the chat reports the turn as running; further annotations sent
	// during the same turn accumulate rather than replace each other.
	const [workingOn, setWorkingOn] = useState<{
		items: HighlightItem[];
		started: boolean;
	}>();

	// Each saved comment goes straight to the agent as its own message.
	// The shimmer starts once the send is accepted.
	const sendQueueRef = useRef(newSendQueue());
	const handleSubmit = (submission: AnnotationSubmission) => {
		if (!composer) {
			return;
		}
		const message = formatAnnotations(submission);
		const items = submission.annotations.map(({ id, element }) => ({
			id,
			selector: element.selector,
			url: submission.page.url,
		}));
		const accepted = enqueueSend(
			sendQueueRef.current,
			composerRef,
			message,
			() =>
				setWorkingOn((current) => ({
					items: [...(current?.items ?? []), ...items],
					started: current?.started ?? false,
				})),
		);
		if (!accepted) {
			toast.error("Too many UI annotations at once; the latest was not sent.", {
				id: "annotation-queue-full",
			});
		}
	};
	const handleRevoke = () => {
		const dropped = dropQueuedSends(sendQueueRef.current);
		if (dropped > 0) {
			toast.error(
				`${dropped} UI ${dropped === 1 ? "annotation was" : "annotations were"} not sent: annotate mode was turned off first.`,
				{ id: "annotation-queue-dropped" },
			);
		}
	};

	const frameUrl = unavailableMessage
		? undefined
		: withAnnotatorParam(url, overlayRequests > 0);
	const bridge = useAnnotatorBridge({
		frameRef,
		frameKey: overlayRequests,
		frameOrigin: frameUrl ? new URL(frameUrl).origin : undefined,
		enabled: !hasPopout && overlayRequests > 0,
		readyTimeoutMs: annotatorReadyTimeoutMs,
		onSubmit: handleSubmit,
		onRevoke: handleRevoke,
	});

	// The chat page's side of the tab's window: learn when one opens and
	// closes, and lend it this chat's composer, under the same gating as
	// the visible input.
	useEffect(() => {
		if (isPopoutWindow) {
			return;
		}
		const channel = new BroadcastChannel(tabPopoutChannelName(tab.id));
		const reply = (message: TabPopoutMessage) => channel.postMessage(message);
		channel.addEventListener("message", (event: MessageEvent<unknown>) => {
			if (!isTabPopoutMessage(event.data)) {
				return;
			}
			const message = event.data;
			switch (message.type) {
				case "popout-opened":
					setHasPopout(true);
					break;
				case "popout-closed":
					setHasPopout(false);
					break;
				case "send": {
					const sent = composer
						? composer.send(message.message)
						: Promise.resolve<ComposerSendResult>("busy");
					sent.then(
						(result) => reply({ type: "send-result", id: message.id, result }),
						(error: unknown) =>
							reply({
								type: "send-result",
								id: message.id,
								result: {
									error: getErrorMessage(
										error,
										"Failed to send UI annotation.",
									),
								},
							}),
					);
					break;
				}
			}
		});
		// A window left open across a reload of this page answers with
		// popout-opened.
		channel.postMessage({ type: "probe" } satisfies TabPopoutMessage);
		return () => channel.close();
	}, [tab.id, composer, isPopoutWindow]);

	// The window renders the same panel and needs the same chat state.
	useEffect(() => {
		if (hasPopout) {
			postToTabPopout(tab.id, { type: "chat-state", isAgentWorking });
		}
	}, [hasPopout, tab.id, isAgentWorking]);

	// Same window geometry as the desktop popout. Naming the window means a
	// second click focuses and reloads the one already open.
	const handlePopout = () => {
		const width = Math.round(screen.availWidth * 0.5);
		const height = Math.round(screen.availHeight * 0.5);
		const left = Math.round((screen.availWidth - width) / 2);
		const top = Math.round((screen.availHeight - height) / 2);
		const opened = open(
			tabPopoutPath(chatId, tab.id),
			tabPopoutChannelName(tab.id),
			`popup,width=${width},height=${height},left=${left},top=${top}`,
		);
		if (!opened) {
			toast.error(
				"The browser blocked the window. Allow popups and try again.",
			);
		}
	};

	const handleBringBack = () => {
		postToTabPopout(tab.id, { type: "bring-back" });
		setHasPopout(false);
	};

	// Opened to annotate in: ask for picking as soon as the overlay loads.
	useEffect(() => {
		if (isPopoutWindow) {
			bridge.setPicking(true);
		}
	}, [isPopoutWindow, bridge.setPicking]);

	// The overlay is requested at most once per attempt: while it is still
	// loading, further clicks only toggle the desired picking state instead
	// of reloading the frame again. Once the frame has left the document
	// the overlay was loaded into, the next click requests it afresh.
	const handleAnnotateClick = () => {
		if (bridge.unavailable) {
			return;
		}
		if (!bridge.ready && !bridge.loading) {
			setOverlayRequests((count) => count + 1);
		}
		bridge.setPicking(!bridge.requested);
	};

	// The overlay owns the drawing; this only tells it what to show. Runs
	// again when the overlay reloads so a pending shimmer is restored.
	useEffect(() => {
		if (!workingOn || !bridge.ready) {
			return;
		}
		if (isAgentWorking) {
			bridge.highlight(workingOn.items);
			if (!workingOn.started) {
				setWorkingOn({ ...workingOn, started: true });
			}
			return;
		}
		if (workingOn.started) {
			bridge.clearHighlights();
			setWorkingOn(undefined);
			return;
		}
		// Sent, but the turn has not begun. If it never does (the send was a
		// no-op, or the turn ended before the status reached us), drop it.
		const timer = setTimeout(() => setWorkingOn(undefined), turnStartGraceMs);
		return () => clearTimeout(timer);
	}, [
		workingOn,
		isAgentWorking,
		bridge.ready,
		bridge.highlight,
		bridge.clearHighlights,
	]);

	// Everything the control shows about annotate mode comes from what the
	// dashboard asked for, never from what the overlay reports: the page
	// can forge or withhold those reports, and submissions are only ever
	// accepted while the request stands, so the button reads "on" whenever
	// they might be. Escape leaves annotate mode from either side, listened
	// for here in case focus is outside the preview.
	useEffect(() => {
		if (!bridge.requested) {
			return;
		}
		const onKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape" && !event.defaultPrevented) {
				bridge.setPicking(false);
			}
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [bridge.requested, bridge.setPicking]);

	// Focus normally stays in the dashboard when the button is clicked, so
	// move it into the frame once the overlay confirms picking.
	useEffect(() => {
		if (bridge.picking) {
			frameRef.current?.focus();
		}
	}, [bridge.picking]);

	const showAnnotate =
		canAnnotate &&
		composer !== undefined &&
		frameUrl !== undefined &&
		!hasPopout;

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex shrink-0 items-center gap-2 border-0 border-b border-solid border-border-default bg-surface-secondary px-2 py-1 text-xs text-content-secondary">
				<NetworkIcon className="size-3.5 shrink-0" />
				<span className="min-w-0 truncate text-content-primary">
					{tab.label}
				</span>
				<div className="flex-1" />
				{showAnnotate && (
					<>
						<Tooltip>
							{/* aria-disabled rather than disabled so the control stays
							 * focusable and the explanation reaches keyboard users. */}
							<TooltipTrigger asChild>
								<Button
									size="icon"
									variant="subtle"
									aria-pressed={bridge.requested}
									aria-label={
										bridge.requested ? "Stop annotating" : "Annotate elements"
									}
									aria-disabled={bridge.unavailable}
									aria-describedby={
										bridge.unavailable ? unavailableReasonId : undefined
									}
									aria-busy={bridge.loading}
									onClick={handleAnnotateClick}
									className={
										bridge.requested
											? "bg-surface-tertiary text-content-primary"
											: "aria-disabled:cursor-not-allowed aria-disabled:text-content-disabled aria-disabled:hover:text-content-disabled"
									}
								>
									<MessageSquarePlusIcon />
								</Button>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								{bridge.unavailable
									? overlayUnavailableReason
									: bridge.requested
										? "Click elements in the preview to annotate them"
										: "Annotate elements in the preview; each comment is sent to the agent"}
							</TooltipContent>
						</Tooltip>
						{bridge.unavailable && (
							<span id={unavailableReasonId} className="sr-only">
								{overlayUnavailableReason}
							</span>
						)}
					</>
				)}
				{unavailableMessage ? (
					<Button
						size="icon"
						variant="subtle"
						disabled
						aria-label="Open port in new tab"
					>
						<ExternalLinkIcon />
					</Button>
				) : canAnnotate && !isPopoutWindow ? (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								aria-label="Open in a separate window"
								aria-pressed={hasPopout}
								onClick={handlePopout}
								className={
									hasPopout
										? "bg-surface-tertiary text-content-primary"
										: undefined
								}
							>
								<ExternalLinkIcon />
							</Button>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							{hasPopout
								? "Annotations from the separate window are sent to this chat"
								: "Open in a separate window; annotations there are sent to this chat"}
						</TooltipContent>
					</Tooltip>
				) : (
					<Button size="icon" variant="subtle" asChild>
						<a
							href={url}
							target="_blank"
							rel="noreferrer"
							aria-label="Open port in new tab"
						>
							<ExternalLinkIcon />
						</a>
					</Button>
				)}
			</div>
			{unavailableMessage ? (
				<div className="flex min-h-0 flex-1 items-center justify-center px-6 text-center text-xs text-content-secondary">
					{unavailableMessage}
				</div>
			) : hasPopout ? (
				<div
					className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 px-6 text-center text-content-secondary"
					role="status"
				>
					<ExternalLinkIcon className="size-8" />
					<span className="text-sm">
						{tab.label} is open in a separate window.
					</span>
					<Button variant="outline" size="sm" onClick={handleBringBack}>
						Bring back
					</Button>
				</div>
			) : (
				<WorkspaceIframe
					key={overlayRequests}
					ref={frameRef}
					src={frameUrl}
					title={tab.label}
					sandbox={previewSandbox}
					onLoad={bridge.frameLoaded}
				/>
			)}
		</div>
	);
};

function withAnnotatorParam(url: string, enabled: boolean): string {
	if (!enabled) {
		return url;
	}
	const next = new URL(url);
	next.searchParams.set(annotatorQueryParam, "1");
	return next.toString();
}

function getUnavailableMessage({
	host,
	agent,
	url,
}: {
	host: string;
	agent: WorkspaceAgent;
	url: string;
}): string | undefined {
	if (host.trim() === "") {
		return "Port previews require a wildcard access URL.";
	}
	if (agent.status !== "connected") {
		return "Port preview will be available once the workspace agent reconnects.";
	}
	if (url === "#") {
		return "The wildcard access URL produced an invalid preview URL. Check the deployment's wildcard access URL configuration.";
	}
	return undefined;
}
