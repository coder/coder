import type {
	WorkspaceAppStatus as APIWorkspaceAppStatus,
	WorkspaceAppStatusState,
} from "api/typesGenerated";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleAlertIcon, CircleCheckIcon } from "lucide-react";
import type { ReactNode } from "react";

const iconByState: Record<WorkspaceAppStatusState, ReactNode> = {
	complete: (
		<CircleCheckIcon className="size-4 shrink-0 text-content-success" />
	),
	failure: <CircleAlertIcon className="size-4 shrink-0 text-content-warning" />,
	working: <Spinner size="sm" className="shrink-0" loading />,
};

export const WorkspaceAppStatus = ({
	status,
}: {
	status: APIWorkspaceAppStatus | null;
}) => {
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
							{iconByState[status.state]}
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
