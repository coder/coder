import {
	ExternalLinkIcon,
	MessageSquarePlusIcon,
	NetworkIcon,
} from "lucide-react";
import {
	useEffect,
	useEffectEvent,
	useId,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { toast } from "sonner";
import { formatAnnotations } from "#/annotator/formatAnnotations";
import {
	type AnnotationSubmission,
	annotatorQueryParam,
} from "#/annotator/protocol";
import { getErrorMessage, isApiError } from "#/api/errors";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { WorkspaceIframe } from "#/modules/apps/WorkspaceAppFrame";
import { portForwardURL } from "#/utils/portForward";
import { useAnnotatorBridge } from "../../hooks/useAnnotatorBridge";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import type { SubmitChatTurnResult } from "../ChatConversation/submitChatTurn";

const sendRetries = 6;
const sendRetryMs = 500;
// Comments accepted from the preview wait to be sent one at a time, so
// they arrive in order. The wait is short: a person makes a comment every
// few seconds, while a page forging submissions could otherwise line up
// chat turns that keep arriving long after annotate mode is off.
const maxPendingSends = 3;
const overlayUnavailableReason =
	"The annotation overlay could not load in this app. It may block external scripts or not serve HTML.";

// Sends one annotation message, retrying briefly while another submission
// is in flight so a quick second comment is delayed rather than dropped.
// Failures are reported to the user here; the result says whether the
// message was accepted. Kept outside the component, with the queue below,
// because the React Compiler does not yet handle loops in try/catch or
// try/finally.
async function deliver(queue: SendQueue, message: string): Promise<boolean> {
	try {
		for (let attempt = 0; attempt < sendRetries; attempt++) {
			const result = await queue.send?.(message);
			if (result === "sent") {
				return true;
			}
			if (result !== "busy") {
				toast.error(
					"Sending messages is unavailable; the UI annotation was not sent.",
				);
				return false;
			}
			await new Promise((resolve) => setTimeout(resolve, sendRetryMs));
		}
		toast.error("The chat is busy; the UI annotation was not sent.");
	} catch (error) {
		// API failures are already displayed by the chat's request error handler.
		if (!isApiError(error)) {
			toast.error(getErrorMessage(error, "Failed to send UI annotation."));
		}
	}
	return false;
}

type SendQueue = {
	send?: (message: string) => Promise<SubmitChatTurnResult> | undefined;
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

// Queues one message behind the others. Returns false when the queue is
// full and the message was not accepted.
function enqueueSend(queue: SendQueue, message: string): boolean {
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
			await deliver(queue, message);
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
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	tab: Extract<UserRightPanelTab, { kind: "port" }>;
	// Shows the annotate control. Requires the chat-ui-annotations
	// experiment so the app proxy injects the overlay.
	canAnnotate?: boolean;
	onSendToolMessage?: (message: string) => Promise<SubmitChatTurnResult>;
	annotatorReadyTimeoutMs?: number;
}> = ({
	workspace,
	agent,
	host,
	tab,
	canAnnotate = false,
	onSendToolMessage,
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
	const send = useEffectEvent((message: string) =>
		onSendToolMessage?.(message),
	);
	const sendQueueRef = useRef(newSendQueue());
	useLayoutEffect(() => {
		const queue = sendQueueRef.current;
		// Retries read the latest sender; disconnect it when the panel unmounts.
		queue.send = (message) => send(message);
		return () => {
			queue.send = undefined;
		};
	}, []);
	const unavailableReasonId = useId();
	// The proxy injects the overlay only on the request that carries the
	// marker param, so each request reloads the frame. Client-side routing
	// inside the app keeps the overlay; a full navigation drops it until
	// the user presses Annotate again.
	const [overlayRequests, setOverlayRequests] = useState(0);

	// Each saved comment goes straight to the agent as its own message.
	const handleSubmit = (submission: AnnotationSubmission) => {
		if (!onSendToolMessage) {
			toast.error(
				"Sending messages is unavailable; the UI annotation was not sent.",
			);
			return;
		}
		const message = formatAnnotations(submission);
		if (!enqueueSend(sendQueueRef.current, message)) {
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
		enabled: overlayRequests > 0,
		readyTimeoutMs: annotatorReadyTimeoutMs,
		onSubmit: handleSubmit,
		onRevoke: handleRevoke,
	});
	const canSendMessage = onSendToolMessage !== undefined;
	const stopAnnotating = useEffectEvent(() => {
		if (bridge.requested) {
			bridge.setPicking(false);
			toast.error(
				"Annotation mode stopped because sending messages is unavailable.",
			);
		}
	});
	useLayoutEffect(() => {
		if (!canSendMessage) {
			stopAnnotating();
		}
	}, [canSendMessage]);

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

	const showAnnotate = canAnnotate && canSendMessage && frameUrl !== undefined;

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
						{/* aria-disabled rather than disabled so the control stays
						 * focusable and the explanation reaches keyboard users. */}
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
			) : (
				<WorkspaceIframe
					key={overlayRequests}
					ref={frameRef}
					src={frameUrl}
					title={tab.label}
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
