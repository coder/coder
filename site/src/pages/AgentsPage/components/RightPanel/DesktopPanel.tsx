import { HandIcon, MousePointer2Icon } from "lucide-react";
import type { FC } from "react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
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
	isControlling: boolean;
	onTakeControl: () => void;
	onReleaseControl: () => void;
}

export const DesktopPanel: FC<DesktopPanelProps> = ({ chatId, isVisible }) => {
	// Delay the VNC connection until the desktop tab is first selected.
	// Once activated, the connection stays alive even when the tab is
	// switched away — mirrors the terminal panel pattern from PR #23231.
	const [activated, setActivated] = useState(false);
	if (isVisible && !activated) {
		setActivated(true);
	}

	const [isControlling, setIsControlling] = useState(false);
	if (!isVisible && isControlling) {
		setIsControlling(false);
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
			isControlling={isControlling}
			onTakeControl={() => setIsControlling(true)}
			onReleaseControl={() => setIsControlling(false)}
		/>
	);
};

export const DesktopPanelView: FC<DesktopPanelViewProps> = ({
	status,
	reconnect,
	attach,
	isControlling,
	onTakeControl,
	onReleaseControl,
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
			{/* "Release Control" button — top-right, only when controlling */}
			{isControlling && (
				<Button
					variant="default"
					size="sm"
					onClick={onReleaseControl}
					className="absolute top-2 right-2 z-20 shadow-xl drop-shadow-lg"
				>
					<HandIcon className="h-4 w-4" />
					Release control
				</Button>
			)}
			{/* VNC container — pointer-events toggled */}
			<div
				ref={(el) => {
					if (el) attach(el);
				}}
				className={cn("h-full w-full", !isControlling && "pointer-events-none")}
			/>
			{/* "Take Control" hover overlay — only when NOT controlling */}
			{!isControlling && (
				<div className="group/desktop absolute inset-0 z-10 flex items-center justify-center bg-black/0 transition-all duration-200 ease-in-out group-hover/desktop:bg-black/40">
					<span className="opacity-0 transition-opacity duration-200 ease-in-out group-hover/desktop:opacity-100">
						<Button
							variant="default"
							size="sm"
							onClick={onTakeControl}
							aria-label="Take control of desktop"
							className="shadow-xl drop-shadow-lg"
						>
							<MousePointer2Icon className="h-4 w-4" />
							Take control
						</Button>
					</span>
				</div>
			)}
		</div>
	);
};
