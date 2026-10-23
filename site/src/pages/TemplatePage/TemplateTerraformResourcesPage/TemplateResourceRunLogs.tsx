import { useQuery } from "react-query";
import { templateResourceRunLogs } from "#/api/queries/templateResources";
import { ProvisionerJobLogs } from "#/components/Logs/ProvisionerJobLogs";

type TemplateResourceRunLogsProps = {
	templateId: string;
	runId: string;
};

/** Shows persisted Terraform output, including logs retained after a failed run. */
export const TemplateResourceRunLogs: React.FC<
	TemplateResourceRunLogsProps
> = ({ templateId, runId }) => {
	const query = useQuery(templateResourceRunLogs(templateId, runId));
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
