import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import { startWorkspace } from "#/api/queries/workspaces";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentStatus,
	WorkspaceStatus,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	type DesktopConnectionStatus,
	useDesktopConnection,
} from "../../hooks/useDesktopConnection";
import { useZoomShortcuts } from "../../hooks/useZoomShortcuts";
import { DesktopToolbar, type ScaleMode } from "./DesktopToolbar";

interface DesktopPanelProps {
	chatId: string;
	workspace: Workspace;
	/** Absent while the workspace is stopped or rebuilding. */
	workspaceAgent: WorkspaceAgent | undefined;
	/** When true the panel is the active sidebar tab. */
	isVisible?: boolean;
}

/** Build statuses from which the workspace can be started again. */
const startableWorkspaceStatuses: readonly WorkspaceStatus[] = [
	"stopped",
	"failed",
	"canceled",
];

// The desktop endpoint rejects any agent that is not connected, so this
// is the precondition for dialing it.
const isDesktopReachable = (
	workspaceStatus: WorkspaceStatus,
	agentStatus: WorkspaceAgentStatus | undefined,
): boolean => workspaceStatus === "running" && agentStatus === "connected";

export const DesktopPanel: FC<DesktopPanelProps> = ({
	chatId,
	workspace,
	workspaceAgent,
	isVisible,
}) => {
	// Delay the VNC connection until the desktop tab is first selected.
	// Once activated, the connection stays alive even when the tab is
	// switched away.
	const [activated, setActivated] = useState(false);
	if (isVisible && !activated) {
		setActivated(true);
	}

	const queryClient = useQueryClient();
	const { mutate: start, isPending: isStartingWorkspace } = useMutation({
		...startWorkspace(workspace, queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to start workspace."));
		},
	});

	const [isControlling, setIsControlling] = useState(false);
	if (!isVisible && isControlling) {
		setIsControlling(false);
	}

	const [scaleMode, setScaleMode] = useState<ScaleMode>("fit");
	const [isPoppedOut, setIsPoppedOut] = useState(false);

	// Gating the connection on the live workspace watch tears the session
	// down when the workspace stops and dials again as soon as the agent
	// reconnects, without manual retries.
	const { status, reconnect, attach } = useDesktopConnection({
		chatId: isPoppedOut ? undefined : chatId,
		activated:
			activated &&
			!isPoppedOut &&
			isDesktopReachable(workspace.latest_build.status, workspaceAgent?.status),
		scaleViewport: scaleMode === "fit",
	});

	useZoomShortcuts(setScaleMode, isVisible);

	// Listen for BroadcastChannel messages from the pop-out window.
	useEffect(() => {
		const channel = new BroadcastChannel(`coder-desktop-${chatId}`);

		channel.addEventListener("message", (event) => {
			if (event.data?.type === "popout-opened") {
				setIsPoppedOut(true);
				setIsControlling(false);
			} else if (event.data?.type === "popout-closed") {
				setIsPoppedOut(false);
			}
		});

		return () => channel.close();
	}, [chatId]);

	const handlePopOut = () => {
		const width = Math.round(screen.availWidth * 0.5);
		const height = Math.round(screen.availHeight * 0.5);
		const left = Math.round((screen.availWidth - width) / 2);
		const top = Math.round((screen.availHeight - height) / 2);
		open(
			`/agents/${chatId}/desktop`,
			`coder-desktop-${chatId}`,
			`popup,width=${width},height=${height},left=${left},top=${top}`,
		);
	};

	const handleBringBack = () => {
		const channel = new BroadcastChannel(`coder-desktop-${chatId}`);
		channel.postMessage({ type: "bring-back" });
		channel.close();
		setIsPoppedOut(false);
	};

	if (isPoppedOut) {
		return (
			<div
				className="flex h-full flex-col items-center justify-center gap-3 text-content-secondary"
				role="status"
			>
				<ExternalLinkIcon className="size-8" />
				<span className="text-sm">Desktop is open in a separate window.</span>
				<Button variant="outline" size="sm" onClick={handleBringBack}>
					Bring back
				</Button>
			</div>
		);
	}

	return (
		<DesktopPanelView
			status={status}
			workspaceStatus={workspace.latest_build.status}
			agentStatus={workspaceAgent?.status}
			onStartWorkspace={() => start({})}
			isStartingWorkspace={isStartingWorkspace}
			reconnect={reconnect}
			attach={attach}
			scaleMode={scaleMode}
			onScaleModeChange={setScaleMode}
			isControlling={isControlling}
			onTakeControl={() => setIsControlling(true)}
			onReleaseControl={() => setIsControlling(false)}
			onPopOut={handlePopOut}
		/>
	);
};

export interface DesktopPanelViewProps {
	status: DesktopConnectionStatus;
	workspaceStatus: WorkspaceStatus;
	agentStatus: WorkspaceAgentStatus | undefined;
	onStartWorkspace: () => void;
	isStartingWorkspace: boolean;
	reconnect: () => void;
	attach: (container: HTMLElement) => void;
	scaleMode: ScaleMode;
	onScaleModeChange: (mode: ScaleMode) => void;
	isControlling: boolean;
	onTakeControl: () => void;
	onReleaseControl: () => void;
	onPopOut?: () => void;
}

export const DesktopPanelView: FC<DesktopPanelViewProps> = ({
	status,
	workspaceStatus,
	agentStatus,
	onStartWorkspace,
	isStartingWorkspace,
	reconnect,
	attach,
	scaleMode,
	onScaleModeChange,
	isControlling,
	onTakeControl,
	onReleaseControl,
	onPopOut,
}) => {
	if (startableWorkspaceStatuses.includes(workspaceStatus)) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-3 text-content-secondary">
				<span className="text-center text-sm">
					The workspace is stopped. Start it to reconnect to the desktop.
				</span>
				<Button
					variant="outline"
					size="sm"
					onClick={onStartWorkspace}
					disabled={isStartingWorkspace}
				>
					<Spinner loading={isStartingWorkspace} />
					Start workspace
				</Button>
			</div>
		);
	}

	if (workspaceStatus === "deleted") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<span className="text-sm">The workspace has been deleted.</span>
			</div>
		);
	}

	if (!isDesktopReachable(workspaceStatus, agentStatus)) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="size-6" />
				<span className="text-sm">
					{workspaceStatus === "running"
						? "Waiting for the workspace agent to connect..."
						: `Workspace is ${workspaceStatus}...`}
				</span>
			</div>
		);
	}

	if (status === "connecting") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="size-6" />
				<span className="text-sm">Connecting to desktop...</span>
			</div>
		);
	}

	if (status === "disconnected") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="size-6" />
				<span className="text-sm">Desktop disconnected. Reconnecting...</span>
			</div>
		);
	}

	if (status === "error") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-3 text-content-secondary">
				<span className="text-center text-sm">
					Failed to connect to the desktop session. The agent may not be
					connected or the desktop environment may not be available.
				</span>
				<Button variant="outline" size="sm" onClick={reconnect}>
					Reconnect
				</Button>
			</div>
		);
	}

	if (status === "idle") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="size-6" />
				<span className="text-sm">Initializing desktop...</span>
			</div>
		);
	}

	// status === "connected"
	return (
		<div className="flex h-full w-full flex-col">
			<DesktopToolbar
				scaleMode={scaleMode}
				onScaleModeChange={onScaleModeChange}
				isControlling={isControlling}
				onTakeControl={onTakeControl}
				onReleaseControl={onReleaseControl}
				onPopOut={onPopOut}
			/>

			<div className="min-h-0 flex-1 overflow-hidden bg-surface-secondary">
				<div
					ref={(el) => {
						if (el) attach(el);
					}}
					className="h-full w-full"
					inert={!isControlling ? true : undefined}
					role="application"
					aria-label={
						isControlling
							? "Remote desktop (interactive)"
							: "Remote desktop (view only, take control to interact)"
					}
				/>
			</div>
		</div>
	);
};
