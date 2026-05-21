import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { DesktopToolbar } from "./components/RightPanel/DesktopToolbar";
import { useDesktopConnection } from "./hooks/useDesktopConnection";

type ScaleMode = "native" | "fit";

const CHANNEL_PREFIX = "coder-desktop-";

export default function DesktopPopoutPage() {
	const { agentId } = useParams() as { agentId: string };
	const [scaleMode, setScaleMode] = useState<ScaleMode>("native");
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

	// Keyboard shortcuts for zoom toggle.
	useEffect(() => {
		const handleKeyDown = (e: KeyboardEvent) => {
			const mod = e.ctrlKey || e.metaKey;
			if (mod && e.key === "0") {
				e.preventDefault();
				setScaleMode("fit");
			} else if (mod && e.key === "1") {
				e.preventDefault();
				setScaleMode("native");
			}
		};
		addEventListener("keydown", handleKeyDown);
		return () => removeEventListener("keydown", handleKeyDown);
	}, []);

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
		<div className="flex h-screen w-screen flex-col overflow-hidden bg-black">
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
				className="min-h-0 flex-1 overflow-hidden"
			/>
		</div>
	);
}
