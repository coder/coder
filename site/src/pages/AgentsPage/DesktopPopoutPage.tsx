import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { DesktopToolbar } from "./components/RightPanel/DesktopToolbar";
import { CHANNEL_PREFIX, type ScaleMode } from "./desktopConstants";
import { useDesktopConnection } from "./hooks/useDesktopConnection";
import { useZoomShortcuts } from "./hooks/useZoomShortcuts";

export default function DesktopPopoutPage() {
	const { agentId } = useParams() as { agentId: string };
	const [scaleMode, setScaleMode] = useState<ScaleMode>("fit");
	const [isControlling, setIsControlling] = useState(false);

	const { status, reconnect, attach } = useDesktopConnection({
		chatId: agentId,
		activated: true,
		scaleViewport: scaleMode === "fit",
	});

	// BroadcastChannel for parent window communication.
	useEffect(() => {
		const channel = new BroadcastChannel(`${CHANNEL_PREFIX}${agentId}`);

		channel.postMessage({ type: "popout-opened" });

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
			handleBeforeUnload();
			removeEventListener("beforeunload", handleBeforeUnload);
			channel.close();
		};
	}, [agentId]);

	useZoomShortcuts(setScaleMode);

	if (status === "idle" || status === "connecting") {
		return (
			<div className="flex h-screen w-screen items-center justify-center bg-surface-primary">
				<div className="flex flex-col items-center gap-2 text-content-secondary">
					<Spinner loading className="h-6 w-6" />
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
						Failed to connect to the desktop session.
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
					<Spinner loading className="h-6 w-6" />
					<span className="text-sm">Desktop disconnected. Reconnecting...</span>
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-screen w-screen flex-col overflow-hidden bg-surface-secondary">
			<DesktopToolbar
				scaleMode={scaleMode}
				onScaleModeChange={setScaleMode}
				isControlling={isControlling}
				onTakeControl={() => setIsControlling(true)}
				onReleaseControl={() => setIsControlling(false)}
				isPoppedOut
			/>
			<div
				ref={(el) => {
					if (el) attach(el);
				}}
				className="min-h-0 flex-1 overflow-hidden bg-surface-secondary"
			/>
		</div>
	);
}
