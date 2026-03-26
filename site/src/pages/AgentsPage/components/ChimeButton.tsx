import { Volume2Icon, VolumeOffIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { getChimeEnabled, setChimeEnabled } from "./AgentDetail/useAgentChime";

export const ChimeButton: FC = () => {
	const [enabled, setEnabled] = useState(getChimeEnabled);

	const handleClick = () => {
		const next = !enabled;
		setEnabled(next);
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
						enabled ? "Mute completion chime" : "Enable completion chime"
					}
					className="h-7 w-7 text-content-secondary hover:text-content-primary"
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
