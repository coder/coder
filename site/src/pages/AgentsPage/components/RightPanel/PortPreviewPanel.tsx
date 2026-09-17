import {
	ExternalLinkIcon,
	MessageSquarePlusIcon,
	NetworkIcon,
} from "lucide-react";
import { type FC, useEffect, useId, useRef, useState } from "react";
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
	useComposer,
} from "../../context/ComposerContext";
import { useAnnotatorBridge } from "../../hooks/useAnnotatorBridge";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";

const sendRetries = 6;
const sendRetryMs = 500;
const overlayUnavailableReason =
	"The annotation overlay could not load in this app. It may block external scripts or not serve HTML.";
// A sent annotation whose turn never starts within this window is
// forgotten rather than left to light up during an unrelated later turn.
const turnStartGraceMs = 15_000;

// Sends one annotation message, retrying briefly while another submission
// is in flight so a quick second comment is delayed rather than dropped.
// Failures are reported to the user here; the result says whether the
// message was accepted. Kept outside the component because the React
// Compiler does not yet handle loops inside try/catch.
async function deliver(
	composer: ComposerHandle,
	message: string,
): Promise<boolean> {
	try {
		for (let attempt = 0; attempt < sendRetries; attempt++) {
			if ((await composer.send(message)) === "sent") {
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

export const PortPreviewPanel: FC<{
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
	annotatorReadyTimeoutMs?: number;
}> = ({
	workspace,
	agent,
	host,
	tab,
	canAnnotate = false,
	isAgentWorking = false,
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
	const unavailableReasonId = useId();
	// The proxy injects the overlay only on the request that carries the
	// marker param, so each request reloads the frame. Client-side routing
	// inside the app keeps the overlay; a full navigation drops it until
	// the user presses Annotate again.
	const [overlayRequests, setOverlayRequests] = useState(0);
	// Elements from annotations sent this turn, highlighted in the preview
	// until the agent's turn ends. `started` guards against a send resolving
	// before the chat reports the turn as running; further annotations sent
	// during the same turn accumulate rather than replace each other.
	const [workingOn, setWorkingOn] = useState<{
		items: HighlightItem[];
		started: boolean;
	}>();

	// Each saved comment goes straight to the agent as its own message.
	// Sends are serialised so quick successive comments arrive in order.
	// The shimmer starts once the send is accepted.
	const sendQueueRef = useRef(Promise.resolve());
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
		sendQueueRef.current = sendQueueRef.current.then(async () => {
			if (await deliver(composer, message)) {
				setWorkingOn((current) => ({
					items: [...(current?.items ?? []), ...items],
					started: current?.started ?? false,
				}));
			}
		});
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
	});

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

	// Escape leaves annotate mode from either side. Focus normally stays in
	// the dashboard when the button is clicked, so move it into the frame
	// once the overlay confirms picking, and also listen here in case it
	// moves back.
	useEffect(() => {
		if (!bridge.picking) {
			return;
		}
		frameRef.current?.focus();
		const onKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape" && !event.defaultPrevented) {
				bridge.setPicking(false);
			}
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [bridge.picking, bridge.setPicking]);

	const showAnnotate =
		canAnnotate && composer !== undefined && frameUrl !== undefined;

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
									aria-pressed={bridge.picking}
									aria-label={
										bridge.picking ? "Stop annotating" : "Annotate elements"
									}
									aria-disabled={bridge.unavailable}
									aria-describedby={
										bridge.unavailable ? unavailableReasonId : undefined
									}
									aria-busy={bridge.loading}
									onClick={handleAnnotateClick}
									className={
										bridge.picking
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
									: bridge.picking
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
