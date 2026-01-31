import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { PauseIcon, PlayIcon } from "lucide-react";
import type { FC } from "react";

type TaskActionButtonProps = {
	action: "pause" | "resume";
	disabled?: boolean;
	loading?: boolean;
	onClick: () => void;
};

const actionConfig = {
	pause: {
		icon: PauseIcon,
		label: "Pause task",
		tooltip: "Pause the task to save resources. You can resume later.",
	},
	resume: {
		icon: PlayIcon,
		label: "Resume task",
		tooltip: "Resuming takes time while the workspace starts.",
	},
} as const;

export const TaskActionButton: FC<TaskActionButtonProps> = ({
	action,
	disabled,
	loading,
	onClick,
}) => {
	const config = actionConfig[action];
	const Icon = config.icon;

	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						size="icon-lg"
						variant="outline"
						disabled={disabled || loading}
						onClick={(e) => {
							e.stopPropagation();
							onClick();
						}}
					>
						<Spinner loading={loading} />
						{!loading && <Icon aria-hidden="true" />}
						<span className="sr-only">{config.label}</span>
					</Button>
				</TooltipTrigger>
				<TooltipContent>{config.tooltip}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
