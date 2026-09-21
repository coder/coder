import type { FC } from "react";
import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	DesktopToolbar,
	type ScaleMode,
} from "./components/RightPanel/DesktopToolbar";
import {
	DesktopWorkspaceState,
	type DesktopWorkspaceStateProps,
	isDesktopReachable,
	useChatDesktopWorkspace,
	useStartDesktopWorkspace,
} from "./components/RightPanel/DesktopWorkspaceState";
import {
	type DesktopConnectionStatus,
	useDesktopConnection,
} from "./hooks/useDesktopConnection";
import { useZoomShortcuts } from "./hooks/useZoomShortcuts";

export default function DesktopPopoutPage() {
	const { agentId } = useParams() as { agentId: string };
	const [scaleMode, setScaleMode] = useState<ScaleMode>("fit");
	const [isControlling, setIsControlling] = useState(false);

	const { workspace, workspaceAgent } = useChatDesktopWorkspace(agentId);
	const { startWorkspace, isStartingWorkspace } =
		useStartDesktopWorkspace(workspace);

	// Same gating as DesktopPanel: dial only while the agent is connected
	// so a stopped workspace tears the session down and a restarted one
	// reconnects on its own.
	const { status, reconnect, attach } = useDesktopConnection({
		chatId: agentId,
		activated:
			workspace !== undefined &&
			isDesktopReachable(workspace.latest_build.status, workspaceAgent?.status),
		scaleViewport: scaleMode === "fit",
	});

	// BroadcastChannel for parent window communication.
	useEffect(() => {
		const channel = new BroadcastChannel(`coder-desktop-${agentId}`);

		channel.postMessage({ type: "popout-opened" });

		// Retry in case the parent's listener registered after this message.
		const retryTimer = setTimeout(() => {
			channel.postMessage({ type: "popout-opened" });
		}, 300);

		channel.addEventListener("message", (event) => {
			if (event.data?.type === "bring-back") {
				close();
			}
		});

		const handleBeforeUnload = () => {
			channel.postMessage({ type: "popout-closed" });
		};
		addEventListener("beforeunload", handleBeforeUnload);

		return () => {
			clearTimeout(retryTimer);
			handleBeforeUnload();
			removeEventListener("beforeunload", handleBeforeUnload);
			channel.close();
		};
	}, [agentId]);

	useZoomShortcuts(setScaleMode);

	return (
		<DesktopPopoutPageView
			status={status}
			workspaceStatus={workspace?.latest_build.status}
			agentStatus={workspaceAgent?.status}
			onStartWorkspace={startWorkspace}
			isStartingWorkspace={isStartingWorkspace}
			reconnect={reconnect}
			attach={attach}
			scaleMode={scaleMode}
			onScaleModeChange={setScaleMode}
			isControlling={isControlling}
			onTakeControl={() => setIsControlling(true)}
			onReleaseControl={() => setIsControlling(false)}
		/>
	);
}

export interface DesktopPopoutPageViewProps
	extends Omit<DesktopWorkspaceStateProps, "workspaceStatus"> {
	status: DesktopConnectionStatus;
	/** Undefined until the chat's workspace has loaded. */
	workspaceStatus: DesktopWorkspaceStateProps["workspaceStatus"] | undefined;
	reconnect: () => void;
	attach: (container: HTMLElement) => void;
	scaleMode: ScaleMode;
	onScaleModeChange: (mode: ScaleMode) => void;
	isControlling: boolean;
	onTakeControl: () => void;
	onReleaseControl: () => void;
}

export const DesktopPopoutPageView: FC<DesktopPopoutPageViewProps> = ({
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
}) => {
	if (
		workspaceStatus !== undefined &&
		!isDesktopReachable(workspaceStatus, agentStatus)
	) {
		return (
			<div className="h-screen w-screen bg-surface-primary">
				<DesktopWorkspaceState
					workspaceStatus={workspaceStatus}
					agentStatus={agentStatus}
					onStartWorkspace={onStartWorkspace}
					isStartingWorkspace={isStartingWorkspace}
				/>
			</div>
		);
	}

	if (status === "idle" || status === "connecting") {
		return (
			<div className="flex h-screen w-screen items-center justify-center bg-surface-primary">
				<div className="flex flex-col items-center gap-2 text-content-secondary">
					<Spinner loading className="size-6" />
					<span className="text-sm">
						{status === "idle"
							? "Initializing desktop..."
							: "Connecting to desktop..."}
					</span>
				</div>
			</div>
		);
	}

	if (status === "error") {
		return (
			<div className="flex h-screen w-screen items-center justify-center bg-surface-primary">
				<div className="flex flex-col items-center gap-3 text-content-secondary">
					<span className="text-center text-sm">
						Failed to connect to the desktop session. The agent may not be
						connected or the desktop environment may not be available.
					</span>
					<Button variant="outline" size="sm" onClick={reconnect}>
						Reconnect
					</Button>
				</div>
			</div>
		);
	}

	if (status === "disconnected") {
		return (
			<div className="flex h-screen w-screen items-center justify-center bg-surface-primary">
				<div className="flex flex-col items-center gap-2 text-content-secondary">
					<Spinner loading className="size-6" />
					<span className="text-sm">Desktop disconnected. Reconnecting...</span>
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-screen w-screen flex-col overflow-hidden bg-surface-secondary">
			<DesktopToolbar
				scaleMode={scaleMode}
				onScaleModeChange={onScaleModeChange}
				isControlling={isControlling}
				onTakeControl={onTakeControl}
				onReleaseControl={onReleaseControl}
				isPoppedOut
			/>
			<div
				ref={(el) => {
					if (el) attach(el);
				}}
				className="min-h-0 flex-1 overflow-hidden bg-surface-secondary"
				inert={!isControlling ? true : undefined}
				role="application"
				aria-label={
					isControlling
						? "Remote desktop (interactive)"
						: "Remote desktop (view only, take control to interact)"
				}
			/>
		</div>
	);
};
