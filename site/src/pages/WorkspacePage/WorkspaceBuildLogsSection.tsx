import type { FC } from "react";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { Loader } from "#/components/Loader/Loader";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { cn } from "#/utils/cn";

interface WorkspaceBuildLogsSectionProps {
	logs?: ProvisionerJobLog[];
}

export const WorkspaceBuildLogsSection: FC<WorkspaceBuildLogsSectionProps> = ({
	logs,
}) => {
	return (
		<div className="rounded-lg border-solid overflow-hidden bg-surface-primary">
			<header
				className={cn(
					"bg-surface-secondary border-solid border-0 border-b",
					"px-2 py-2 pl-6 flex items-center rounded-t-lg",
					"text-sm",
				)}
			>
				Build logs
			</header>
			<div className="h-[400px] overflow-y-auto">
				{logs ? (
					<WorkspaceBuildLogs
						sticky
						logs={logs}
						className="rounded-none border-none"
					/>
				) : (
					<div className="flex items-center justify-center w-full h-full">
						<Loader />
					</div>
				)}
			</div>
		</div>
	);
};
