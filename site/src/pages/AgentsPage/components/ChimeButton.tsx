import { Volume2Icon, VolumeOffIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { getChimeEnabled, setChimeEnabled } from "../utils/chime";

interface ChimeButtonProps {
	enabled?: boolean;
	onToggle?: () => void;
}

export const ChimeButton: FC<ChimeButtonProps> = ({ enabled, onToggle }) => {
	const [internalEnabled, setInternalEnabled] = useState(getChimeEnabled);
	const isControlled = enabled !== undefined && onToggle !== undefined;
	const isEnabled = isControlled ? enabled : internalEnabled;

	const handleClick = () => {
		if (isControlled) {
			onToggle();
			return;
		}
		const next = !internalEnabled;
		setInternalEnabled(next);
		setChimeEnabled(next);
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
					className="h-7 w-7 text-content-secondary hover:text-content-primary"
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
