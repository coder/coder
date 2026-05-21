import {
	ExternalLinkIcon,
	HandIcon,
	MaximizeIcon,
	MinimizeIcon,
	MousePointer2Icon,
} from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ScaleMode = "native" | "fit";

interface DesktopToolbarProps {
	scaleMode: ScaleMode;
	onScaleModeChange: (mode: ScaleMode) => void;
	isControlling: boolean;
	onTakeControl: () => void;
	onReleaseControl: () => void;
	onPopOut?: () => void;
	isPoppedOut?: boolean;
}

export const DesktopToolbar: FC<DesktopToolbarProps> = ({
	scaleMode,
	onScaleModeChange,
	isControlling,
	onTakeControl,
	onReleaseControl,
	onPopOut,
	isPoppedOut,
}) => {
	return (
		<div
			className="flex h-8 shrink-0 items-center justify-between border-0 border-b border-solid border-border-default bg-surface-primary px-1.5"
			role="toolbar"
			aria-label="Desktop controls"
		>
			{/* Left: Take/Release control */}
			<Button
				variant="subtle"
				size="sm"
				onClick={isControlling ? onReleaseControl : onTakeControl}
				className="h-6 gap-1.5 px-2 text-xs"
			>
				{isControlling ? (
					<>
						<HandIcon className="size-3.5" />
						Release control
					</>
				) : (
					<>
						<MousePointer2Icon className="size-3.5" />
						Take control
					</>
				)}
			</Button>

			{/* Right: Zoom + Pop-out */}
			<div className="flex items-center gap-1">
				{/* Zoom toggle */}
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							onClick={() =>
								onScaleModeChange(scaleMode === "native" ? "fit" : "native")
							}
							aria-label={
								scaleMode === "native"
									? "Switch to fit-to-window (Ctrl+0)"
									: "Switch to 100% zoom (Ctrl+1)"
							}
							className="h-6 w-6 text-content-secondary hover:text-content-primary"
						>
							{scaleMode === "native" ? (
								<MinimizeIcon className="size-3.5" />
							) : (
								<MaximizeIcon className="size-3.5" />
							)}
						</Button>
					</TooltipTrigger>
					<TooltipContent>
						{scaleMode === "native"
							? "Fit to window (Ctrl+0)"
							: "100% zoom (Ctrl+1)"}
					</TooltipContent>
				</Tooltip>

				{/* Pop-out button */}
				{onPopOut && !isPoppedOut && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="subtle"
								size="icon"
								onClick={onPopOut}
								aria-label="Open desktop in new window"
								className="h-6 w-6 text-content-secondary hover:text-content-primary"
							>
								<ExternalLinkIcon className="size-3.5" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>Open in new window</TooltipContent>
					</Tooltip>
				)}
			</div>
		</div>
	);
};
