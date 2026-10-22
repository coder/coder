import { useQuery } from "react-query";
import { networkPolicyRunLogs } from "#/api/queries/networkPolicies";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Logs } from "#/components/Logs/Logs";

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
			{Boolean(query.error) && (
				<>
					<ErrorAlert error={query.error} />
					<Button variant="outline" onClick={() => void query.refetch()}>
						Retry logs
					</Button>
				</>
			)}
			{query.isLoading && <Loader />}
			{query.data?.length === 0 && (
				<p className="text-sm text-content-secondary">
					No provisioner logs yet.
				</p>
			)}
			{query.data && query.data.length > 0 && (
				<Logs
					lines={query.data.map((log) => ({
						id: log.id,
						time: log.created_at,
						level: log.log_level,
						output: log.output,
						sourceId: log.log_source,
					}))}
				/>
			)}
		</div>
	);
};
