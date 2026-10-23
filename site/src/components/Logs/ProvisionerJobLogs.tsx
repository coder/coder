import type { ProvisionerJobLog } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Logs } from "./Logs";

type ProvisionerJobLogsProps = {
	logs: readonly ProvisionerJobLog[] | undefined;
	error: unknown;
	isLoading: boolean;
	onRetry: () => void;
	emptyMessage: string;
};

/** Keeps persisted output visible when a background log request fails. */
export const ProvisionerJobLogs: React.FC<ProvisionerJobLogsProps> = ({
	logs,
	error,
	isLoading,
	onRetry,
	emptyMessage,
}) => (
	<>
		{Boolean(error) && (
			<>
				<ErrorAlert error={error} />
				<Button variant="outline" onClick={onRetry}>
					Retry logs
				</Button>
			</>
		)}
		{isLoading && <Loader />}
		{logs?.length === 0 && (
			<p className="text-sm text-content-secondary">{emptyMessage}</p>
		)}
		{logs && logs.length > 0 && (
			<Logs
				lines={logs.map((log) => ({
					id: log.id,
					time: log.created_at,
					level: log.log_level,
					output: log.output,
					sourceId: log.log_source,
				}))}
			/>
		)}
	</>
);
