import type { ProvisionerJobLog } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import type { FC } from "react";

interface WorkspaceBuildLogsSectionProps {
	logs?: ProvisionerJobLog[];
}

export const WorkspaceBuildLogsSection: FC<WorkspaceBuildLogsSectionProps> = ({
	logs,
}) => {
	return (
		<div className="rounded-lg border border-solid border-zinc-700 overflow-hidden bg-surface-secondary">
			<header className="bg-surface-secondary border-0 border-b border-solid border-zinc-700 p-2 pl-6 text-[13px] font-semibold flex items-center rounded-t-lg">
				Build logs
			</header>
			<div className="h-100 overflow-y-auto">
				{logs ? (
					<WorkspaceBuildLogs
						sticky
						logs={logs}
						className="border-0 rounded-none"
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
