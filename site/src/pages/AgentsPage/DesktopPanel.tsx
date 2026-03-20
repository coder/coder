import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { type FC, useEffect, useRef } from "react";
import {
	type UseDesktopConnectionResult,
	useDesktopConnection,
} from "./useDesktopConnection";

interface DesktopPanelProps {
	chatId: string;
	isExpanded: boolean;
	/** Optional override for the desktop connection. Used in stories. */
	connectionOverride?: UseDesktopConnectionResult;
}

export const DesktopPanel: FC<DesktopPanelProps> = ({
	chatId,
	isExpanded: _isExpanded,
	connectionOverride,
}) => {
	// When an override is provided, pass undefined chatId to prevent
	// the real hook from attempting any WebSocket connections.
	const hookResult = useDesktopConnection({
		chatId: connectionOverride ? undefined : chatId,
	});
	const { status, connect, disconnect, attach } =
		connectionOverride ?? hookResult;
	const containerRef = useRef<HTMLDivElement | null>(null);

	const attachToContainer = (el: HTMLDivElement | null) => {
		containerRef.current = el;
		if (el) {
			attach(el);
		}
	};

	// Connect on mount, disconnect on unmount. This drives the
	// visibility-based lifecycle: DesktopPanel is only rendered
	// when the Desktop tab is active, so mounting/unmounting
	// naturally starts and stops the WebSocket connection.
	useEffect(() => {
		connect();
		return () => {
			disconnect();
		};
	}, [connect, disconnect]);

	// Re-attach when status changes to connected (e.g., after reconnect).
	useEffect(() => {
		if (status === "connected" && containerRef.current) {
			attach(containerRef.current);
		}
	}, [status, attach]);

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
				<span className="text-sm">
					Failed to connect to the desktop session.
				</span>
				<Button variant="outline" size="sm" onClick={() => connect()}>
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
	return <div ref={attachToContainer} className="h-full w-full" />;
};
