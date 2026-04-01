import { FullscreenIcon } from "lucide-react";
import type { FC } from "react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDesktopConnection } from "../../hooks/useDesktopConnection";
import type { UseDesktopModeResult } from "../../hooks/useDesktopMode";

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
	/** Desktop landscape mode state from useDesktopMode. */
	desktopMode?: UseDesktopModeResult;
}

export interface DesktopPanelViewProps {
	status: DesktopConnectionStatus;
	reconnect: () => void;
	attach: (container: HTMLElement) => void;
	desktopMode?: UseDesktopModeResult;
}

export const DesktopPanel: FC<DesktopPanelProps> = ({
	chatId,
	isVisible,
	desktopMode,
}) => {
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
		<DesktopPanelView
			status={status}
			reconnect={reconnect}
			attach={attach}
			desktopMode={desktopMode}
		/>
	);
};

export const DesktopPanelView: FC<DesktopPanelViewProps> = ({
	status,
	reconnect,
	attach,
	desktopMode,
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
		<div className="relative h-full w-full">
			<div
				ref={(el) => {
					if (el) attach(el);
				}}
				className="h-full w-full"
			/>
			{/* Landscape button — mobile only, overlaid on the VNC
			   canvas. Tapping rotates the device to landscape so
			   the desktop view fills the screen. */}
			{desktopMode?.isSupported && !desktopMode.isLandscape && (
				<Button
					variant="outline"
					size="sm"
					onClick={desktopMode.enterLandscape}
					aria-label="Enter landscape mode"
					className="absolute bottom-3 right-3 z-10 gap-1.5 bg-surface-primary/90 backdrop-blur-sm shadow-md md:hidden"
				>
					<FullscreenIcon className="h-3.5 w-3.5" /> Landscape
				</Button>
			)}
		</div>
	);
};
