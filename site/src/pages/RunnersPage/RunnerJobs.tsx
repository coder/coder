import {
	CheckCircle2Icon,
	CircleXIcon,
	GitBranchIcon,
	LoaderCircleIcon,
} from "lucide-react";
import { Badge } from "#/components/Badge/Badge";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { humanDurationShort } from "#/utils/time";
import type { DemoJob } from "./demoData";

type RunnerJobsProps = { jobs: DemoJob[]; now: number };

/** Displays the first eight newest-first jobs relative to `now`, a Unix timestamp in milliseconds. */
export const RunnerJobs: React.FC<RunnerJobsProps> = ({ jobs, now }) => (
	<section className="min-w-0 rounded-lg border">
		<div className="flex items-center justify-between border-b p-5">
			<h2 className="m-0 text-base font-semibold">Recent jobs</h2>
			<span className="text-xs text-content-secondary">
				Latest {Math.min(jobs.length, 8)} jobs
			</span>
		</div>
		{jobs.length === 0 ? (
			<EmptyState
				isCompact
				className="py-10"
				message="No jobs yet"
				description="Jobs will appear here when a workflow uses this runner's label."
			/>
		) : (
			<Table
				aria-label="Recent runner jobs"
				className="min-w-180"
				wrapperClassName="rounded-b-lg"
			>
				<TableHeader>
					<TableRow>
						<TableHead>Job</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>Branch</TableHead>
						<TableHead>Queue</TableHead>
						<TableHead>Duration</TableHead>
						<TableHead>Started</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody className="[&>tr>*:first-child]:border-l-0 [&>tr>*:last-child]:border-r-0 [&>tr:last-child>*]:border-b-0">
					{jobs.slice(0, 8).map((job) => {
						const minutesAgo = Math.max(
							1,
							Math.round((now - job.startedAt) / 60_000),
						);
						return (
							<TableRow key={job.id}>
								<TableHead scope="row">
									<div className="flex flex-col gap-1">
										<span className="text-content-primary">{job.name}</span>
										<span className="text-xs font-normal text-content-secondary">
											{job.workflow}{" "}
											<span className="text-content-secondary">#{job.id}</span>
										</span>
									</div>
								</TableHead>
								<TableCell>
									<Badge
										variant={
											job.status === "Succeeded"
												? "green"
												: job.status === "Failed"
													? "destructive"
													: "info"
										}
									>
										{job.status === "Succeeded" ? (
											<CheckCircle2Icon />
										) : job.status === "Failed" ? (
											<CircleXIcon />
										) : (
											<LoaderCircleIcon />
										)}
										{job.status}
									</Badge>
								</TableCell>
								<TableCell>
									<span className="flex max-w-32 items-center gap-1.5">
										<GitBranchIcon className="size-3 shrink-0" />
										<span className="truncate" title={job.branch}>
											{job.branch}
										</span>
									</span>
								</TableCell>
								<TableCell className="whitespace-nowrap">
									{humanDurationShort(job.queueSeconds * 1000)}
								</TableCell>
								<TableCell className="whitespace-nowrap">
									{humanDurationShort(
										job.status === "In progress"
											? now - job.startedAt
											: job.durationSeconds * 1000,
									)}
								</TableCell>
								<TableCell className="whitespace-nowrap">
									<time
										dateTime={new Date(job.startedAt).toISOString()}
										title={new Date(job.startedAt).toLocaleString("en-US")}
									>
										{minutesAgo < 60
											? `${minutesAgo}m ago`
											: `${Math.floor(minutesAgo / 60)}h ago`}
									</time>
								</TableCell>
							</TableRow>
						);
					})}
				</TableBody>
			</Table>
		)}
	</section>
);
