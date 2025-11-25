import type { WorkspaceAppStatus as APIWorkspaceAppStatus } from "api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import capitalize from "lodash/capitalize";
import { AppStatusStateIcon } from "modules/apps/AppStatusStateIcon";

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

	const message = status.message || capitalize(status.state);
	return (
		<div className="flex flex-col text-content-secondary">
			<Tooltip>
				<TooltipTrigger asChild>
					<div className="flex items-center gap-2">
						<AppStatusStateIcon
							latest
							disabled={disabled}
							state={status.state}
						/>
						<span className="whitespace-nowrap max-w-72 overflow-hidden text-ellipsis text-sm text-content-primary font-medium">
							{message}
						</span>
					</div>
				</TooltipTrigger>
				<TooltipContent>{message}</TooltipContent>
			</Tooltip>
			<span className="text-xs first-letter:uppercase block pl-6">
				{status.state}
			</span>
		</div>
	);
};
