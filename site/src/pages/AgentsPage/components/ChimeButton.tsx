import { Volume2Icon, VolumeOffIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type ChimeButtonProps = {
	enabled: boolean;
	onToggle: () => void;
};

export const ChimeButton: FC<ChimeButtonProps> = ({ enabled, onToggle }) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					onClick={onToggle}
					aria-label={
						enabled ? "Mute completion chime" : "Enable completion chime"
					}
					className="size-7 text-content-secondary hover:text-content-primary"
				>
					{enabled ? (
						<Volume2Icon className="text-content-success" />
					) : (
						<VolumeOffIcon className="text-content-secondary" />
					)}
				</Button>
			</TooltipTrigger>
			<TooltipContent>
				{enabled ? "Disable completion sound" : "Enable completion sound"}
			</TooltipContent>
		</Tooltip>
	);
};
