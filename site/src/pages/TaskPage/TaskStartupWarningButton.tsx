import type { WorkspaceAgentLifecycle } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Link } from "components/Link/Link";
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
	lifecycleState?: WorkspaceAgentLifecycle | null;
};

export const TaskStartupWarningButton: FC<TaskStartupWarningButtonProps> = ({
	lifecycleState,
}) => {
	switch (lifecycleState) {
		case "start_error":
			return <ErrorScriptButton />;
		case "start_timeout":
			return <TimeoutScriptButton />;
		default:
			return null;
	}
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
			label="Startup error"
			errorMessage="startup script has exited with an error"
		/>
	);
};

const TimeoutScriptButton: FC = () => {
	return (
		<StartupWarningButtonBase
			label="Startup timeout"
			errorMessage="startup script has timed out"
		/>
	);
};
