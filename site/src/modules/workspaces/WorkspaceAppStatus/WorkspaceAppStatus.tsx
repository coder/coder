import type { WorkspaceAppStatus as APIWorkspaceAppStatus } from "api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { AppStatusStateIcon } from "modules/apps/AppStatusStateIcon";
import { cn } from "utils/cn";

type WorkspaceAppStatusProps = {
	status: APIWorkspaceAppStatus | null;
	disabled?: boolean;
};

export const WorkspaceAppStatus = ({
	status,
	disabled,
}: WorkspaceAppStatusProps) => {
	if (!status) {
		return (
			<span className="text-content-disabled text-sm">
				-<span className="sr-only">No activity</span>
			</span>
		);
	}

	return (
		<div className="flex flex-col text-content-secondary">
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<div className="flex items-center gap-2">
							<AppStatusStateIcon
								latest
								disabled={disabled}
								state={status.state}
								className={cn({
									"text-content-disabled": disabled,
								})}
							/>
							<span className="whitespace-nowrap max-w-72 overflow-hidden text-ellipsis text-sm text-content-primary font-medium">
								{status.message}
							</span>
						</div>
					</TooltipTrigger>
					<TooltipContent>{status.message}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
			<span className="text-xs first-letter:uppercase block pl-6">
				{status.state}
			</span>
		</div>
	);
};
