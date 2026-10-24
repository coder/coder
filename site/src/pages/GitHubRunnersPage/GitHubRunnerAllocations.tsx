import { useQuery } from "react-query";
import { githubRunners } from "#/api/queries/githubRunners";
import type { GitHubRunnerPool } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";

type GitHubRunnerAllocationsProps = {
	organization: string;
	pool: GitHubRunnerPool;
};

/** Retains deleted allocations so admins can inspect cleanup and job results. */
export const GitHubRunnerAllocations: React.FC<
	GitHubRunnerAllocationsProps
> = ({ organization, pool }) => {
	const runnersQuery = useQuery(githubRunners(organization, pool.id));
	return (
		<section aria-label={`Runners for ${pool.name}`} className="space-y-4 pt-4">
			<h2 className="text-lg font-semibold">Runners for {pool.name}</h2>
			<p className="text-sm text-content-secondary">
				Workflow label: <code>runs-on: {pool.name}</code>
			</p>
			{runnersQuery.error && (
				<div className="space-y-3">
					<ErrorAlert error={runnersQuery.error} />
					<Button variant="outline" onClick={() => void runnersQuery.refetch()}>
						Retry runners
					</Button>
				</div>
			)}
			{!runnersQuery.data && !runnersQuery.error && <Loader />}
			{runnersQuery.data?.length === 0 && (
				<EmptyState
					isCompact
					message="No runners yet"
					description="Runner workspaces appear here when GitHub assigns jobs to this pool."
				/>
			)}
			{runnersQuery.data && runnersQuery.data.length > 0 && (
				<Table aria-label={`Runner allocations for ${pool.name}`}>
					<TableHeader>
						<TableRow>
							<TableHead>Runner</TableHead>
							<TableHead>State</TableHead>
							<TableHead>Result</TableHead>
							<TableHead>Workspace ID</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{runnersQuery.data.map((runner) => (
							<TableRow key={runner.id}>
								<TableCell>
									<span className="font-mono text-xs">{runner.name}</span>
									{runner.last_error && (
										<p className="mb-0 text-xs text-content-destructive break-words">
											{runner.last_error}
										</p>
									)}
								</TableCell>
								<TableCell>
									<Badge variant="outline">{runner.state}</Badge>
								</TableCell>
								<TableCell>{runner.job_result || "Pending"}</TableCell>
								<TableCell className="font-mono text-xs">
									{runner.workspace_id ===
									"00000000-0000-0000-0000-000000000000"
										? "Not created"
										: runner.workspace_id || "Not created"}
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}
		</section>
	);
};
