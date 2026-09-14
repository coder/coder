import {
	ExternalLinkIcon,
	MessageSquarePlusIcon,
	NetworkIcon,
} from "lucide-react";
import { type FC, useRef, useState } from "react";
import { toast } from "sonner";
import { formatAnnotations } from "#/annotator/formatAnnotations";
import {
	type AnnotationSubmission,
	annotatorQueryParam,
} from "#/annotator/protocol";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { WorkspaceIframe } from "#/modules/apps/WorkspaceAppFrame";
import { portForwardURL } from "#/utils/portForward";
import { useComposer } from "../../context/ComposerContext";
import { useAnnotatorBridge } from "../../hooks/useAnnotatorBridge";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";

export const annotationsFileName = "ui-annotations.md";

export const PortPreviewPanel: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	tab: Extract<UserRightPanelTab, { kind: "port" }>;
	// Shows the annotate control. Requires the chat-ui-annotations
	// experiment so the app proxy injects the overlay.
	canAnnotate?: boolean;
}> = ({ workspace, agent, host, tab, canAnnotate = false }) => {
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
	// The proxy injects the overlay only on the request that carries the
	// marker param, so each request reloads the frame. Client-side routing
	// inside the app keeps the overlay; a full navigation drops it until
	// the user presses Annotate again.
	const [overlayRequests, setOverlayRequests] = useState(0);

	// Annotations are attached rather than sent: the frame is a third-party
	// app that could forge a submission, so the user reviews the draft and
	// presses send themselves.
	const handleSubmit = (submission: AnnotationSubmission) => {
		if (!composer) {
			return;
		}
		composer.attach([
			new File([formatAnnotations(submission)], annotationsFileName, {
				type: "text/markdown",
			}),
		]);
		const count = submission.annotations.length;
		toast.success(
			`Attached ${count} UI annotation${count === 1 ? "" : "s"} to your message.`,
		);
	};

	const frameUrl = unavailableMessage
		? undefined
		: withAnnotatorParam(url, overlayRequests > 0);
	const bridge = useAnnotatorBridge({
		frameRef,
		frameKey: overlayRequests,
		frameOrigin: frameUrl ? new URL(frameUrl).origin : undefined,
		enabled: overlayRequests > 0,
		onSubmit: handleSubmit,
	});

	const handleAnnotateClick = () => {
		if (!bridge.ready) {
			setOverlayRequests((count) => count + 1);
		}
		bridge.setPicking(!bridge.picking);
	};

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
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								aria-pressed={bridge.picking}
								aria-label={
									bridge.picking ? "Stop annotating" : "Annotate elements"
								}
								onClick={handleAnnotateClick}
								className={
									bridge.picking
										? "relative bg-surface-tertiary text-content-primary"
										: "relative"
								}
							>
								<MessageSquarePlusIcon />
								{bridge.count > 0 && (
									<span className="absolute -right-0.5 -top-0.5 flex size-3.5 items-center justify-center rounded-full bg-content-link text-2xs font-semibold text-surface-primary">
										{bridge.count}
									</span>
								)}
							</Button>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							{bridge.picking
								? "Click elements in the preview to annotate them"
								: "Annotate elements in the preview and attach them to your message"}
						</TooltipContent>
					</Tooltip>
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
