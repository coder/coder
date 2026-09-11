import RFB from "@novnc/novnc/lib/rfb";
import type { FC, Ref } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { watchWorkspaceAgentDesktop } from "#/api/api";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";

type DesktopStatus = "connecting" | "connected" | "disconnected";

interface WorkspaceDesktopViewProps {
	status: DesktopStatus;
	onReconnect: () => void;
	/** Element noVNC renders its canvas into. */
	canvasRef?: Ref<HTMLDivElement>;
}

/**
 * WorkspaceDesktopView is the presentational half of the desktop: the noVNC
 * canvas host plus a status overlay with a reconnect action.
 */
export const WorkspaceDesktopView: FC<WorkspaceDesktopViewProps> = ({
	status,
	onReconnect,
	canvasRef,
}) => {
	return (
		<div className="relative h-full w-full bg-surface-secondary">
			<div
				ref={canvasRef}
				className="h-full w-full"
				data-testid="desktop-canvas"
			/>
			{status !== "connected" && (
				<div className="absolute inset-0 flex flex-col items-center justify-center gap-4 bg-surface-primary/80 text-content-secondary">
					{status === "connecting" ? (
						<>
							<Spinner loading />
							<span>Connecting to desktop</span>
						</>
					) : (
						<>
							<span>Disconnected from desktop</span>
							<Button variant="outline" onClick={onReconnect}>
								Reconnect
							</Button>
						</>
					)}
				</div>
			)}
		</div>
	);
};

interface WorkspaceDesktopProps {
	agentId: string;
}

/**
 * WorkspaceDesktop renders a noVNC session to a workspace agent's built-in
 * desktop. It fills its parent, scales the remote framebuffer to fit, and
 * offers a reconnect button when the session drops.
 */
export const WorkspaceDesktop: FC<WorkspaceDesktopProps> = ({ agentId }) => {
	const containerRef = useRef<HTMLDivElement>(null);
	const [status, setStatus] = useState<DesktopStatus>("connecting");
	const [attempt, setAttempt] = useState(0);

	const reconnect = useCallback(() => {
		setStatus("connecting");
		setAttempt((n) => n + 1);
	}, []);

	useEffect(() => {
		const container = containerRef.current;
		if (!container) {
			return;
		}
		let disposed = false;

		const rfb = new RFB(container, watchWorkspaceAgentDesktop(agentId), {
			shared: true,
		});
		rfb.scaleViewport = true;
		rfb.resizeSession = false;
		rfb.focusOnClick = true;

		rfb.addEventListener("connect", () => {
			if (!disposed) {
				setStatus("connected");
			}
		});
		rfb.addEventListener("disconnect", () => {
			if (!disposed) {
				setStatus("disconnected");
			}
		});

		return () => {
			disposed = true;
			try {
				rfb.disconnect();
			} catch {
				// The socket may already be closed.
			}
		};
	}, [agentId, attempt]);

	return (
		<WorkspaceDesktopView
			status={status}
			onReconnect={reconnect}
			canvasRef={containerRef}
		/>
	);
};
