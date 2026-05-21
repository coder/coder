import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { useEffect, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { useDesktopConnection } from "../../hooks/useDesktopConnection";
import { useDesktopPanel } from "../ChatElements/tools/DesktopPanelContext";
import { type DesktopApp, DesktopToolbar } from "./DesktopToolbar";

/** Default desktop apps available in the VNC session. */
const DEFAULT_DESKTOP_APPS: readonly DesktopApp[] = [
	{
		name: "Chrome",
		icon: "/icon/google.svg",
		command:
			"google-chrome-stable --no-sandbox --disable-gpu --disable-dev-shm-usage --no-first-run --no-default-browser-check --window-size=1280,720",
	},
];

/**
 * Launch a desktop app inside the VNC session by sending the command
 * through a short-lived reconnecting PTY websocket.
 */
function launchDesktopApp(app: DesktopApp, agentId: string) {
	const cmd = `DISPLAY=:1 nohup ${app.command} > /dev/null 2>&1 &\n`;
	const reconnect = crypto.randomUUID();
	const proto = location.protocol === "https:" ? "wss:" : "ws:";
	const url = `${proto}//${location.host}/api/v2/workspaceagents/${agentId}/pty?reconnect=${reconnect}&height=1&width=80`;

	try {
		const ws = new WebSocket(url);
		ws.binaryType = "arraybuffer";
		ws.addEventListener("open", () => {
			// The reconnecting PTY expects raw text data.
			ws.send(new TextEncoder().encode(cmd));
			setTimeout(() => ws.close(), 2000);
		});
		ws.addEventListener("error", () => {
			try {
				ws.close();
			} catch {
				// ignore
			}
		});
	} catch {
		// Silently fail; the user can retry from the menu.
	}
}

type ScaleMode = "native" | "fit";

type DesktopConnectionStatus =
	| "idle"
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

const CHANNEL_PREFIX = "coder-desktop-";

interface DesktopPanelProps {
	chatId: string;
	/** When true the panel is the active sidebar tab. */
	isVisible?: boolean;
}

export const DesktopPanel: FC<DesktopPanelProps> = ({ chatId, isVisible }) => {
	// Delay the VNC connection until the desktop tab is first selected.
	// Once activated, the connection stays alive even when the tab is
	// switched away.
	const [activated, setActivated] = useState(false);
	if (isVisible && !activated) {
		setActivated(true);
	}

	const [isControlling, setIsControlling] = useState(false);
	if (!isVisible && isControlling) {
		setIsControlling(false);
	}

	const [scaleMode, setScaleMode] = useState<ScaleMode>("native");
	const [isPoppedOut, setIsPoppedOut] = useState(false);

	const { status, reconnect, attach } = useDesktopConnection({
		chatId: isPoppedOut ? undefined : chatId,
		activated: activated && !isPoppedOut,
		scaleViewport: scaleMode === "fit",
	});

	const { agent, workspace } = useDesktopPanel();

	// Keyboard shortcuts for zoom toggle.
	useEffect(() => {
		if (!isVisible) return;
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
	}, [isVisible]);

	// Listen for BroadcastChannel messages from the pop-out window.
	useEffect(() => {
		const channel = new BroadcastChannel(`${CHANNEL_PREFIX}${chatId}`);

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
		const channel = new BroadcastChannel(`${CHANNEL_PREFIX}${chatId}`);
		channel.postMessage({ type: "bring-back" });
		channel.close();
		setIsPoppedOut(false);
	};

	if (isPoppedOut) {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-3 text-content-secondary">
				<ExternalLinkIcon className="h-8 w-8" />
				<span className="text-sm">Desktop is open in a separate window.</span>
				<Button variant="outline" size="sm" onClick={handleBringBack}>
					Bring back
				</Button>
			</div>
		);
	}

	const handleLaunchDesktopApp = (app: DesktopApp) => {
		if (agent) {
			launchDesktopApp(app, agent.id);
		}
	};

	return (
		<DesktopPanelView
			status={status}
			reconnect={reconnect}
			attach={attach}
			scaleMode={scaleMode}
			onScaleModeChange={setScaleMode}
			isControlling={isControlling}
			onTakeControl={() => setIsControlling(true)}
			onReleaseControl={() => setIsControlling(false)}
			onPopOut={handlePopOut}
			agent={agent}
			workspace={workspace}
			desktopApps={DEFAULT_DESKTOP_APPS}
			onLaunchDesktopApp={handleLaunchDesktopApp}
		/>
	);
};

export interface DesktopPanelViewProps {
	status: DesktopConnectionStatus;
	reconnect: () => void;
	attach: (container: HTMLElement) => void;
	scaleMode: ScaleMode;
	onScaleModeChange: (mode: ScaleMode) => void;
	isControlling: boolean;
	onTakeControl: () => void;
	onReleaseControl: () => void;
	onPopOut?: () => void;
	agent?: import("#/api/typesGenerated").WorkspaceAgent;
	workspace?: import("#/api/typesGenerated").Workspace;
	desktopApps?: readonly DesktopApp[];
	onLaunchDesktopApp?: (app: DesktopApp) => void;
}

export const DesktopPanelView: FC<DesktopPanelViewProps> = ({
	status,
	reconnect,
	attach,
	scaleMode,
	onScaleModeChange,
	isControlling,
	onTakeControl,
	onReleaseControl,
	onPopOut,
	agent,
	workspace,
	desktopApps,
	onLaunchDesktopApp,
}) => {
	const scrollRef = useRef<HTMLDivElement>(null);

	// Scroll-wheel panning: translate wheel events into scroll offsets
	// when the desktop is at native (100%) zoom and overflows the panel.
	// Uses capture phase so the handler fires before noVNC can forward
	// the event to the remote desktop (which otherwise triggers XFCE
	// workspace switching).
	useEffect(() => {
		const el = scrollRef.current;
		if (!el) return;

		const handleWheel = (e: WheelEvent) => {
			if (scaleMode !== "native") return;
			e.preventDefault();
			e.stopPropagation();
			el.scrollLeft += e.deltaX || e.deltaY;
			el.scrollTop += e.deltaY;
		};

		el.addEventListener("wheel", handleWheel, {
			passive: false,
			capture: true,
		});
		return () =>
			el.removeEventListener("wheel", handleWheel, { capture: true });
	}, [scaleMode]);

	if (status === "connecting") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="h-6 w-6" />
				<span className="text-sm">Connecting to desktop...</span>
			</div>
		);
	}

	if (status === "disconnected") {
		return (
			<div className="flex h-full flex-col items-center justify-center gap-2 text-content-secondary">
				<Spinner loading className="h-6 w-6" />
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
				<Spinner loading className="h-6 w-6" />
				<span className="text-sm">Initializing desktop...</span>
			</div>
		);
	}

	// status === "connected"
	return (
		<div className="flex h-full w-full flex-col">
			<DesktopToolbar
				agent={agent}
				workspace={workspace}
				scaleMode={scaleMode}
				onScaleModeChange={onScaleModeChange}
				isControlling={isControlling}
				onTakeControl={onTakeControl}
				onReleaseControl={onReleaseControl}
				onPopOut={onPopOut}
				desktopApps={desktopApps}
				onLaunchDesktopApp={onLaunchDesktopApp}
			/>
			{/* Scrollable container for native zoom panning */}
			<div
				ref={scrollRef}
				className={cn(
					"min-h-0 flex-1",
					scaleMode === "native" ? "overflow-auto" : "overflow-hidden",
				)}
			>
				<div
					ref={(el) => {
						if (el) attach(el);
					}}
					className={cn(
						"h-full w-full",
						!isControlling && "pointer-events-none",
					)}
				/>
			</div>
		</div>
	);
};
