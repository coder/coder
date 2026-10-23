import { useQuery } from "react-query";
import { networkPolicyRunLogs } from "#/api/queries/networkPolicies";
import { ProvisionerJobLogs } from "#/components/Logs/ProvisionerJobLogs";

type NetworkPolicyRunLogsProps = {
	organization: string;
	policyId: string;
	runId: string;
};

/** Polls persisted output while retaining cached logs when a refresh fails. */
export const NetworkPolicyRunLogs: React.FC<NetworkPolicyRunLogsProps> = ({
	organization,
	policyId,
	runId,
}) => {
	const query = useQuery(networkPolicyRunLogs(organization, policyId, runId));
	return (
		<div className="space-y-3">
			<h3 className="text-sm font-semibold">Provisioner logs</h3>
			<ProvisionerJobLogs
				logs={query.data}
				error={query.error}
				isLoading={query.isLoading}
				onRetry={() => void query.refetch()}
				emptyMessage="No provisioner logs yet."
			/>
		</div>
	);
};
