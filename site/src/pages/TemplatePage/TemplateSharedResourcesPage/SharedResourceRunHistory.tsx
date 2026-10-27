import { useState } from "react";
import type { TemplateResources } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { SharedResourceRunLogs } from "./SharedResourceRunLogs";

type SharedResourceRunHistoryProps = {
	resources: TemplateResources;
};

/** Preserves an explicitly selected historical run while newer jobs arrive. */
export const SharedResourceRunHistory: React.FC<
	SharedResourceRunHistoryProps
> = ({ resources }) => {
	const [selectedRunId, setSelectedRunId] = useState<string>();
	const runs = resources.runs.toSorted((a, b) =>
		b.created_at.localeCompare(a.created_at),
	);
	const selectedRun = runs.find((run) => run.id === selectedRunId) ?? runs[0];
	return (
		<section aria-label="Run history" className="space-y-4">
			<h2 className="text-xl font-semibold">Run history</h2>
			{runs.length === 0 ? (
				<p className="text-sm text-content-secondary">
					No runs yet. Plan a saved revision to preview its changes.
				</p>
			) : (
				<>
					<Table aria-label="Terraform runs">
						<TableHeader>
							<TableRow>
								<TableHead>Action</TableHead>
								<TableHead>Revision</TableHead>
								<TableHead>Created</TableHead>
								<TableHead>Job status</TableHead>
								<TableHead>Logs</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{runs.map((run) => (
								<TableRow key={run.id}>
									<TableCell className="capitalize">{run.action}</TableCell>
									<TableCell className="font-mono">
										{run.revision_id.slice(0, 8)}
									</TableCell>
									<TableCell>{run.created_at}</TableCell>
									<TableCell>{run.job.status}</TableCell>
									<TableCell>
										<Button
											variant="outline"
											size="sm"
											aria-label={`View logs for ${run.action} ${run.id}`}
											aria-pressed={selectedRun?.id === run.id}
											onClick={() => setSelectedRunId(run.id)}
										>
											View logs
										</Button>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
					{selectedRun && (
						<div className="space-y-3">
							<p className="text-sm text-content-secondary">
								{selectedRun.action} · {selectedRun.id}
							</p>
							{selectedRun.job.error && (
								<ErrorAlert error={{ message: selectedRun.job.error }} />
							)}
							<SharedResourceRunLogs
								templateId={resources.template_id}
								runId={selectedRun.id}
							/>
						</div>
					)}
				</>
			)}
		</section>
	);
};
