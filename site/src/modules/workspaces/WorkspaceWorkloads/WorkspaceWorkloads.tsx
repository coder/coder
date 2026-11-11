import { useId } from "react";
import type { Workload, WorkloadStatus } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { CopyableValue } from "#/components/CopyableValue/CopyableValue";
import { Loader } from "#/components/Loader/Loader";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { formatDateTime, relativeTime } from "#/utils/time";

const statusDisplay: Record<
	WorkloadStatus,
	{ label: string; variant: StatusIndicatorProps["variant"] }
> = {
	pending: { label: "Pending", variant: "pending" },
	starting: { label: "Starting", variant: "pending" },
	running: { label: "Running", variant: "success" },
	disconnected: { label: "Disconnected", variant: "warning" },
};

type WorkspaceWorkloadsProps = {
	workspaceName: string;
	/** Undefined while the first request is in flight. */
	workloads: readonly Workload[] | undefined;
	error: unknown;
};

/**
 * Lists the daemons started in a workspace at runtime with `coder run`.
 */
export const WorkspaceWorkloads: React.FC<WorkspaceWorkloadsProps> = ({
	workspaceName,
	workloads,
	error,
}) => {
	const headingId = useId();

	return (
		<section
			aria-labelledby={headingId}
			className="flex max-w-full flex-col rounded-lg border border-border bg-surface-primary text-sm shadow-md overflow-clip"
		>
			<header className="flex flex-col gap-1 px-8 pt-4 pb-4 leading-normal">
				<h3
					id={headingId}
					className="m-0 text-base font-semibold text-content-primary"
				>
					Workloads
				</h3>
				<p className="m-0 text-sm text-content-secondary">
					Daemons started in this workspace at runtime. Each one has its own
					identity and can only use its sponsor's permissions within its scopes.
				</p>
			</header>

			{Boolean(error) && (
				<div className="px-8 pb-4">
					<ErrorAlert error={error} />
				</div>
			)}

			{workloads === undefined && !error && <Loader className="p-8" />}

			{workloads?.length === 0 && (
				<p className="m-0 border-t border-border px-8 py-6 text-content-secondary">
					No workloads are running. Start one with{" "}
					<code className="font-mono text-content-primary">
						coder run {workspaceName} -- &lt;command&gt;
					</code>
					.
				</p>
			)}

			{workloads !== undefined && workloads.length > 0 && (
				<Table aria-label="Workloads">
					<TableHeader>
						<TableRow>
							<TableHead className="pl-8">Name</TableHead>
							<TableHead>Status</TableHead>
							<TableHead>Command</TableHead>
							<TableHead>Identity</TableHead>
							<TableHead>Sponsor</TableHead>
							<TableHead>Scopes</TableHead>
							<TableHead className="pr-8">Launched</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{workloads.map((workload) => {
							const status = statusDisplay[workload.status];
							return (
								<TableRow key={workload.id}>
									<TableCell className="pl-8 font-semibold text-content-primary">
										{workload.name}
									</TableCell>
									<TableCell className="whitespace-nowrap">
										<StatusIndicator size="sm" variant={status.variant}>
											<StatusIndicatorDot />
											{status.label}
										</StatusIndicator>
									</TableCell>
									<TableCell>
										{workload.command ? (
											<code className="font-mono whitespace-pre-wrap break-all text-content-primary">
												{workload.command}
											</code>
										) : (
											"None"
										)}
									</TableCell>
									<TableCell>
										{workload.identity ? (
											<CopyableValue
												value={workload.identity}
												className="font-mono break-all"
											>
												{workload.identity}
											</CopyableValue>
										) : (
											"Not assigned yet"
										)}
									</TableCell>
									<TableCell className="whitespace-nowrap">
										{workload.sponsor_username}
									</TableCell>
									<TableCell>
										{workload.scopes.length > 0 ? (
											<code className="font-mono text-content-primary">
												{workload.scopes.join(", ")}
											</code>
										) : (
											"None"
										)}
									</TableCell>
									<TableCell
										data-pixel="ignore"
										className="pr-8 whitespace-nowrap"
									>
										<span className="block text-content-primary first-letter:uppercase">
											{relativeTime(workload.created_at)}
										</span>
										<span className="block">
											{formatDateTime(workload.created_at)}
										</span>
									</TableCell>
								</TableRow>
							);
						})}
					</TableBody>
				</Table>
			)}
		</section>
	);
};
