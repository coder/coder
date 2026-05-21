import { type FC, useEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { chat } from "#/api/queries/chats";
import { workspaceById } from "#/api/queries/workspaces";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { getWorkspaceAgent } from "./components/ChatConversation/chatHelpers";
import { DesktopToolbar } from "./components/RightPanel/DesktopToolbar";
import { useDesktopConnection } from "./hooks/useDesktopConnection";

type ScaleMode = "native" | "fit";

const CHANNEL_PREFIX = "coder-desktop-";

export default function DesktopPopoutPage() {
	const { agentId } = useParams() as { agentId: string };
	const [scaleMode, setScaleMode] = useState<ScaleMode>("native");
	const [isControlling, setIsControlling] = useState(false);

	const chatQuery = useQuery(chat(agentId));
	const workspaceId = chatQuery.data?.workspace_id;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const workspace = workspaceQuery.data;
	const workspaceAgent = getWorkspaceAgent(workspace, undefined);

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
				agent={workspaceAgent}
				workspace={workspace}
				scaleMode={scaleMode}
				onScaleModeChange={setScaleMode}
				isControlling={isControlling}
				onTakeControl={() => setIsControlling(true)}
				onReleaseControl={() => setIsControlling(false)}
				isPoppedOut
			/>
			<VncContainer attach={attach} />
		</div>
	);
}

/**
 * VNC container that blocks wheel events from reaching noVNC
 * (prevents XFCE workspace switching on scroll).
 */
const VncContainer: FC<{
	attach: (el: HTMLElement) => void;
}> = ({ attach }) => {
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const el = containerRef.current;
		if (!el) return;

		const handleWheel = (e: WheelEvent) => {
			e.preventDefault();
			e.stopPropagation();
		};

		el.addEventListener("wheel", handleWheel, {
			passive: false,
			capture: true,
		});
		return () =>
			el.removeEventListener("wheel", handleWheel, { capture: true });
	}, []);

	return (
		<div
			ref={(el) => {
				(
					containerRef as React.MutableRefObject<HTMLDivElement | null>
				).current = el;
				if (el) attach(el);
			}}
			className="min-h-0 flex-1 overflow-hidden"
		/>
	);
};
