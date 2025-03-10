import { provisionerJobs } from "api/queries/organizations";
import type { ProvisionerJob } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	ChevronDownIcon,
	ChevronRightIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { relativeTime } from "utils/time";
import { CancelJobButton } from "./CancelJobButton";
import { DataGrid } from "./DataGrid";
import { JobStatusIndicator } from "./JobStatusIndicator";
import { Tag, Tags, TruncateTags } from "./Tags";

type ProvisionerJobsPageProps = {
	orgId: string;
};

export const ProvisionerJobsPage: FC<ProvisionerJobsPageProps> = ({
	orgId,
}) => {
	const {
		data: jobs,
		isLoadingError,
		refetch,
	} = useQuery(provisionerJobs(orgId));

	return (
		<section className="flex flex-col gap-8">
			<h2 className="sr-only">Provisioner jobs</h2>
			<p className="text-sm text-content-secondary m-0 mt-2">
				Provisioner Jobs are the individual tasks assigned to Provisioners when
				the workspaces are being built.{" "}
				<Link href={docs("/admin/provisioners")}>View docs</Link>
			</p>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Created</TableHead>
						<TableHead>Type</TableHead>
						<TableHead>Template</TableHead>
						<TableHead>Tags</TableHead>
						<TableHead>Status</TableHead>
						<TableHead />
					</TableRow>
				</TableHeader>
				<TableBody>
					{jobs ? (
						jobs.length > 0 ? (
							jobs.map((j) => <JobRow key={j.id} job={j} />)
						) : (
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState message="No provisioner jobs found" />
								</TableCell>
							</TableRow>
						)
					) : isLoadingError ? (
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState
									message="Error loading the provisioner jobs"
									cta={<Button onClick={() => refetch()}>Retry</Button>}
								/>
							</TableCell>
						</TableRow>
					) : (
						<TableRow>
							<TableCell colSpan={999}>
								<Loader />
							</TableCell>
						</TableRow>
					)}
				</TableBody>
			</Table>
		</section>
	);
};

type JobRowProps = {
	job: ProvisionerJob;
};

const JobRow: FC<JobRowProps> = ({ job }) => {
	const metadata = job.metadata;
	const [isOpen, setIsOpen] = useState(false);

	return (
		<>
			<TableRow key={job.id}>
				<TableCell>
					<button
						className={cn([
							"flex items-center gap-1 p-0 bg-transparent border-0 text-inherit text-xs cursor-pointer",
							"transition-colors hover:text-content-primary font-medium whitespace-nowrap",
							isOpen && "text-content-primary",
						])}
						type="button"
						onClick={() => {
							setIsOpen((v) => !v);
						}}
					>
						{isOpen ? (
							<ChevronDownIcon className="size-icon-sm p-0.5" />
						) : (
							<ChevronRightIcon className="size-icon-sm p-0.5" />
						)}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						<span className="[&:first-letter]:uppercase">
							{relativeTime(new Date(job.created_at))}
						</span>
					</button>
				</TableCell>
				<TableCell>
					<Badge size="sm">{job.type}</Badge>
				</TableCell>
				<TableCell>
					{job.metadata.template_name ? (
						<div className="flex items-center gap-1 whitespace-nowrap">
							<Avatar
								variant="icon"
								src={metadata.template_icon}
								fallback={
									metadata.template_display_name || metadata.template_name
								}
							/>
							{metadata.template_display_name ?? metadata.template_name}
						</div>
					) : (
						<span className="whitespace-nowrap">Not linked</span>
					)}
				</TableCell>
				<TableCell>
					<TruncateTags tags={job.tags} />
				</TableCell>
				<TableCell>
					<JobStatusIndicator job={job} />
				</TableCell>
				<TableCell className="text-right">
					<CancelJobButton job={job} />
				</TableCell>
			</TableRow>

			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						{job.status === "failed" && (
							<div
								className={cn([
									"inline-flex items-center gap-2 rounded border border-solid border-boder p-2",
									"text-content-primary bg-surface-secondary mb-4",
								])}
							>
								<TriangleAlertIcon className="text-content-destructive size-icon-sm p-0.5" />
								<span className="[&:first-letter]:uppercase">{job.error}</span>
							</div>
						)}
						<DataGrid>
							<dt>Job ID:</dt>
							<dd>{job.id}</dd>

							<dt>Available provisioners:</dt>
							<dd>
								{job.available_workers
									? JSON.stringify(job.available_workers)
									: "[]"}
							</dd>

							<dt>Completed by provisioner:</dt>
							<dd>{job.worker_id}</dd>

							<dt>Associated workspace:</dt>
							<dd>{job.metadata.workspace_name ?? "null"}</dd>

							<dt>Creation time:</dt>
							<dd>{job.created_at}</dd>

							<dt>Queue:</dt>
							<dd>
								{job.queue_position}/{job.queue_size}
							</dd>

							<dt>Tags:</dt>
							<dd>
								<Tags>
									{Object.entries(job.tags).map(([key, value]) => (
										<Tag key={key} label={key} value={value} />
									))}
								</Tags>
							</dd>
						</DataGrid>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
