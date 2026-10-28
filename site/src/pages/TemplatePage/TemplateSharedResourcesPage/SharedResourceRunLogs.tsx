import { useQuery } from "react-query";
import { foundationRunLogs } from "#/api/queries/foundations";
import { ProvisionerJobLogs } from "#/components/Logs/ProvisionerJobLogs";

type SharedResourceRunLogsProps = {
	organization: string;
	foundationId: string;
	runId: string;
};

/** Shows persisted Terraform output, including logs retained after a failed run. */
export const SharedResourceRunLogs: React.FC<SharedResourceRunLogsProps> = ({
	organization,
	foundationId,
	runId,
}) => {
	const query = useQuery(foundationRunLogs(organization, foundationId, runId));
	return (
		<section aria-label="Job logs" className="space-y-3">
			<h3 className="text-base font-semibold">Job logs</h3>
			<ProvisionerJobLogs
				logs={query.data}
				error={query.error}
				isLoading={query.isLoading}
				onRetry={() => void query.refetch()}
				emptyMessage="No job logs yet."
			/>
		</section>
	);
};
