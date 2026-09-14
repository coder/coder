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
import { useComposerAttachments } from "../../context/ComposerAttachmentsContext";
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
	const composer = useComposerAttachments();
	// Once the overlay is requested it stays injected for the life of the
	// tab (the proxy remembers it in a host-only cookie); only picking mode
	// toggles from here on.
	const [overlayRequested, setOverlayRequested] = useState(false);

	const handleSubmit = (submission: AnnotationSubmission) => {
		const file = new File(
			[formatAnnotations(submission)],
			annotationsFileName,
			{
				type: "text/markdown",
			},
		);
		composer.attach([file]);
		const count = submission.annotations.length;
		toast.success(
			`Attached ${count} UI annotation${count === 1 ? "" : "s"} to your message.`,
		);
	};

	const frameUrl = unavailableMessage
		? undefined
		: withAnnotatorParam(url, overlayRequested);
	const bridge = useAnnotatorBridge({
		frameRef,
		frameOrigin: frameUrl ? new URL(frameUrl).origin : undefined,
		onSubmit: handleSubmit,
	});

	const handleAnnotateClick = () => {
		setOverlayRequested(true);
		bridge.setPicking(!bridge.picking);
	};

	const showAnnotate =
		canAnnotate && composer.canAttach && frameUrl !== undefined;

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
				<WorkspaceIframe ref={frameRef} src={frameUrl} title={tab.label} />
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
