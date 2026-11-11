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
import { Switch } from "#/components/Switch/Switch";
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
	starting: { label: "Starting", variant: "pending" },
	running: { label: "Running", variant: "success" },
	unknown: { label: "Unknown", variant: "warning" },
	exited: { label: "Exited", variant: "inactive" },
	removed: { label: "Removed", variant: "inactive" },
	stopped: { label: "Stopped", variant: "inactive" },
	lost: { label: "Lost", variant: "warning" },
	failed: { label: "Failed to start", variant: "failed" },
};

const workloadStatus = (workload: Workload) => {
	const status = statusDisplay[workload.status];
	if (workload.exit_code === undefined) {
		return status;
	}
	return {
		label: `${status.label} (${workload.exit_code})`,
		variant:
			workload.status === "exited" && workload.exit_code !== 0
				? "failed"
				: status.variant,
	} satisfies typeof status;
};

type WorkspaceWorkloadsProps = {
	workspaceName: string;
	/** Undefined while the first request is in flight. */
	workloads: readonly Workload[] | undefined;
	error: unknown;
	/** Whether workloads that are over are listed too. */
	showFinished: boolean;
	onShowFinishedChange?: (show: boolean) => void;
};

/**
 * Lists the workloads of a workspace: commands that run in sandboxes inside
 * it, started with `coder run`, and on request the ones that are over.
 */
export const WorkspaceWorkloads: React.FC<WorkspaceWorkloadsProps> = ({
	workspaceName,
	workloads,
	error,
	showFinished,
	onShowFinishedChange,
}) => {
	const headingId = useId();
	const switchId = useId();

	return (
		<section
			aria-labelledby={headingId}
			className="flex max-w-full flex-col rounded-lg border border-border bg-surface-primary text-sm shadow-md overflow-clip"
		>
			<header className="flex flex-wrap items-start justify-between gap-4 px-8 pt-4 pb-4 leading-normal">
				<div className="flex flex-col gap-1">
					<h3
						id={headingId}
						className="m-0 text-base font-semibold text-content-primary"
					>
						Workloads
					</h3>
					<p className="m-0 text-sm text-content-secondary">
						Commands that run in sandboxes inside this workspace. Each one has
						its own identity and can only use its sponsor's permissions within
						its scopes.
					</p>
				</div>
				<div className="flex items-center gap-2 pt-1">
					<Switch
						id={switchId}
						size="sm"
						checked={showFinished}
						onCheckedChange={onShowFinishedChange}
					/>
					<label htmlFor={switchId} className="text-sm text-content-secondary">
						Show finished
					</label>
				</div>
			</header>

			{Boolean(error) && (
				<div className="px-8 pb-4">
					<ErrorAlert error={error} />
				</div>
			)}

			{workloads === undefined && !error && <Loader className="p-8" />}

			{workloads?.length === 0 && (
				<p className="m-0 border-t border-border px-8 py-6 text-content-secondary">
					{showFinished
						? "No workloads have run in this workspace. Start one with "
						: "No workloads are running. Start one with "}
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
							const status = workloadStatus(workload);
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
										<code className="font-mono whitespace-pre-wrap break-all text-content-primary">
											{workload.command}
										</code>
									</TableCell>
									<TableCell>
										<CopyableValue
											value={workload.identity}
											className="font-mono break-all"
										>
											{workload.identity}
										</CopyableValue>
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
										{workload.finished_at && (
											<span className="block">
												until {formatDateTime(workload.finished_at)}
											</span>
										)}
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
