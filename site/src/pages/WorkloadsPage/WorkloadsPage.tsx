import { useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { sponsoredWorkloads } from "#/api/queries/workspaces";
import { Margins } from "#/components/Margins/Margins";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { WorkspaceWorkloads } from "#/modules/workspaces/WorkspaceWorkloads/WorkspaceWorkloads";
import { pageTitle } from "#/utils/page";

/**
 * Lists the viewer's workloads in every workspace. A workload that had a
 * workspace of its own is still listed after that workspace is gone.
 */
const WorkloadsPage: React.FC = () => {
	const { experiments } = useDashboard();
	const [showFinished, setShowFinished] = useState(true);
	const workloadsQuery = useQuery({
		...sponsoredWorkloads(showFinished),
		enabled: experiments.includes("runtime-workloads"),
		refetchInterval: 5_000,
		placeholderData: keepPreviousData,
	});

	return (
		<>
			<title>{pageTitle("Workloads")}</title>
			<Margins className="py-8">
				<WorkspaceWorkloads
					workloads={workloadsQuery.data}
					error={workloadsQuery.error}
					showFinished={showFinished}
					onShowFinishedChange={setShowFinished}
				/>
			</Margins>
		</>
	);
};

export default WorkloadsPage;
