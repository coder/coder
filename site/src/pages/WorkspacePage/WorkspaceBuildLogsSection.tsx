import { cn } from "cn";
import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { Loader } from "#/components/Loader/Loader";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";

type WorkspaceBuildLogsSectionProps = {
	logs?: ProvisionerJobLog[];
};

export const WorkspaceBuildLogsSection: React.FC<
	WorkspaceBuildLogsSectionProps
> = ({ logs }) => {
	return (
		<div className="rounded-lg border border-solid overflow-hidden bg-surface-primary">
			<header
				className={cn(
					"bg-surface-secondary border-solid border-0 border-b",
					"px-2 py-2 pl-6 flex items-center",
					"text-sm",
				)}
			>
				Build logs
			</header>
			<ScrollArea
				className="h-[400px]"
				// Radix wraps content in display: table, which lets long log lines
				// widen the viewport instead of using the logs' own x scrolling.
				viewportClassName="[&>div]:block!"
			>
				{logs ? (
					<WorkspaceBuildLogs
						sticky
						logs={logs}
						className="rounded-none border-none"
					/>
				) : (
					<div className="flex items-center justify-center w-full h-[400px]">
						<Loader />
					</div>
				)}
			</ScrollArea>
		</div>
	);
};
