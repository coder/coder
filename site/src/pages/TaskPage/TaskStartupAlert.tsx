import Link from "@mui/material/Link";
import type { Task } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { docs } from "utils/docs";

type TaskStartupWarningButtonProps = {
	task: Task;
};

export const TaskStartupWarningButton: FC<TaskStartupWarningButtonProps> = ({
	task,
}) => {
	const lifecycleState = task.workspace_agent_lifecycle;

	if (!lifecycleState) {
		return null;
	}

	if (lifecycleState === "start_error") {
		return <ErrorScriptButton />;
	}

	if (lifecycleState === "start_timeout") {
		return <TimeoutScriptButton />;
	}

	return null;
};

type StartupWarningButtonBaseProps = {
	label: string;
	errorMessage: string;
};

const StartupWarningButtonBase: FC<StartupWarningButtonBaseProps> = ({
	label,
	errorMessage,
}) => {
	return (
		<TooltipProvider delayDuration={250}>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						variant="outline"
						size="sm"
						className="border-amber-500 text-amber-600 dark:border-amber-600 dark:text-amber-400"
					>
						<TriangleAlertIcon />
						{label}
					</Button>
				</TooltipTrigger>
				<TooltipContent className="max-w-sm bg-surface-secondary p-4">
					<p className="m-0 text-sm font-normal text-content-primary leading-snug">
						A workspace{" "}
						<Link
							href={docs(
								"/admin/templates/troubleshooting#startup-script-exited-with-an-error",
							)}
							target="_blank"
							rel="noreferrer"
						>
							{errorMessage}
						</Link>
						. We recommend{" "}
						<Link
							href={docs(
								"/admin/templates/troubleshooting#startup-script-issues",
							)}
							target="_blank"
							rel="noreferrer"
						>
							debugging the startup script
						</Link>{" "}
						because{" "}
						<Link
							href={docs(
								"/admin/templates/troubleshooting#your-workspace-may-be-incomplete",
							)}
							target="_blank"
							rel="noreferrer"
						>
							your workspace may be incomplete
						</Link>
						.
					</p>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

const ErrorScriptButton: FC = () => {
	return (
		<StartupWarningButtonBase
			label="Startup Error"
			errorMessage="startup script has exited with an error"
		/>
	);
};

const TimeoutScriptButton: FC = () => {
	return (
		<StartupWarningButtonBase
			label="Startup Timeout"
			errorMessage="startup script has timed out"
		/>
	);
};
