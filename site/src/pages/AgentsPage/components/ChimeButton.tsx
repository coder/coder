import { Volume2Icon, VolumeOffIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useStorage } from "#/hooks/useStorage";
import { chimeOnCompletionStorage } from "../utils/chime";

interface ChimeButtonProps {
	enabled?: boolean;
	onToggle?: () => void;
}

export const ChimeButton: FC<ChimeButtonProps> = ({ enabled, onToggle }) => {
	const [storedEnabled, setStoredEnabled] = useStorage(
		chimeOnCompletionStorage,
	);
	const isControlled = enabled !== undefined && onToggle !== undefined;
	const isEnabled = isControlled ? enabled : storedEnabled;

	const handleClick = () => {
		if (isControlled) {
			onToggle();
			return;
		}
		setStoredEnabled(!storedEnabled);
	};

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					onClick={handleClick}
					aria-label={
						isEnabled ? "Mute completion chime" : "Enable completion chime"
					}
					className="size-7 text-content-secondary hover:text-content-primary"
				>
					{isEnabled ? (
						<Volume2Icon className="text-content-success" />
					) : (
						<VolumeOffIcon className="text-content-secondary" />
					)}
				</Button>
			</TooltipTrigger>
			<TooltipContent>
				{isEnabled ? "Disable completion sound" : "Enable completion sound"}
			</TooltipContent>
		</Tooltip>
	);
};
