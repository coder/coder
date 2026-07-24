import {
	ExternalLinkIcon,
	HandIcon,
	MaximizeIcon,
	MousePointer2Icon,
	ScalingIcon,
} from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
export type ScaleMode = "native" | "fit";

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
			className="flex h-8 shrink-0 items-center justify-end gap-1 border-0 border-b border-solid border-border-default bg-surface-primary px-1.5"
			role="group"
			aria-label="Desktop controls"
		>
			{/* Take/Release control */}
			<Button
				variant="subtle"
				size="sm"
				onClick={isControlling ? onReleaseControl : onTakeControl}
				aria-pressed={isControlling}
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

			{/* Zoom toggle */}
			<Button
				variant="subtle"
				size="sm"
				onClick={() =>
					onScaleModeChange(scaleMode === "native" ? "fit" : "native")
				}
				aria-label={
					scaleMode === "native"
						? "Zoom to fit (Ctrl+0)"
						: "Zoom to 100% (Ctrl+1)"
				}
				className="h-6 gap-1.5 px-2 text-xs"
			>
				{scaleMode === "native" ? (
					<>
						<ScalingIcon className="size-3.5" />
						Zoom to fit
					</>
				) : (
					<>
						<MaximizeIcon className="size-3.5" />
						Zoom to 100%
					</>
				)}
			</Button>

			{/* Detach button */}
			{onPopOut && !isPoppedOut && (
				<Button
					variant="subtle"
					size="sm"
					onClick={onPopOut}
					aria-label="Detach desktop to new window"
					className="h-6 gap-1.5 px-2 text-xs"
				>
					<ExternalLinkIcon className="size-3.5" />
					Detach
				</Button>
			)}
		</div>
	);
};
