import type { FC } from "react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDesktopConnection } from "../../hooks/useDesktopConnection";

type DesktopConnectionStatus =
	| "idle"
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

interface DesktopPanelProps {
	chatId: string;
	/** When true the panel is the active sidebar tab. */
	isVisible?: boolean;
}

export interface DesktopPanelViewProps {
	status: DesktopConnectionStatus;
	reconnect: () => void;
	attach: (container: HTMLElement) => void;
}

export const DesktopPanel: FC<DesktopPanelProps> = ({ chatId, isVisible }) => {
	// Delay the VNC connection until the desktop tab is first selected.
	// Once activated, the connection stays alive even when the tab is
	// switched away — mirrors the terminal panel pattern from PR #23231.
	const [activated, setActivated] = useState(false);
	if (isVisible && !activated) {
		setActivated(true);
	}

	const { status, reconnect, attach } = useDesktopConnection({
		chatId,
		activated,
	});
	return (
		<DesktopPanelView status={status} reconnect={reconnect} attach={attach} />
	);
};

export const DesktopPanelView: FC<DesktopPanelViewProps> = ({
	status,
	reconnect,
	attach,
}) => {
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
		<div
			ref={(el) => {
				if (el) attach(el);
			}}
			className="h-full w-full"
		/>
	);
};
