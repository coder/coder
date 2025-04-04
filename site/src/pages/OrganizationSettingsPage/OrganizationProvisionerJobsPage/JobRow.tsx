import type { ProvisionerJob } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { TableCell, TableRow } from "components/Table/Table";
import {
	ChevronDownIcon,
	ChevronRightIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { relativeTime } from "utils/time";
import { CancelJobButton } from "./CancelJobButton";
import { JobStatusIndicator } from "modules/provisioners/JobStatusIndicator";
import {
	ProvisionerTag,
	ProvisionerTags,
	ProvisionerTruncateTags,
} from "modules/provisioners/ProvisionerTags";
import { Button } from "components/Button/Button";

type JobRowProps = {
	job: ProvisionerJob;
};

export const JobRow: FC<JobRowProps> = ({ job }) => {
	const metadata = job.metadata;
	const [isOpen, setIsOpen] = useState(false);
	const queue = {
		size: job.queue_size,
		position: job.queue_position,
	};

	return (
		<>
			<TableRow key={job.id}>
				<TableCell>
					<Button
						variant="subtle"
						size="sm"
						className={cn([
							isOpen && "text-content-primary",
							"p-0 h-auto min-w-0 align-middle",
						])}
						onClick={() => {
							setIsOpen((v) => !v);
						}}
					>
						{isOpen ? <ChevronDownIcon /> : <ChevronRightIcon />}
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						<span className="block first-letter:uppercase">
							{relativeTime(new Date(job.created_at))}
						</span>
					</Button>
				</TableCell>
				<TableCell>
					<Badge size="sm">{job.type}</Badge>
				</TableCell>
				<TableCell>
					<div className="flex items-center gap-1 whitespace-nowrap">
						<Avatar
							variant="icon"
							src={metadata.template_icon}
							fallback={
								metadata.template_display_name || metadata.template_name
							}
						/>
						{metadata.template_display_name || metadata.template_name}
					</div>
				</TableCell>
				<TableCell>
					<ProvisionerTruncateTags tags={job.tags} />
				</TableCell>
				<TableCell>
					<JobStatusIndicator status={job.status} queue={queue} />
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
						<dl
							className={cn([
								"text-xs text-content-secondary",
								"m-0 grid grid-cols-[auto_1fr] gap-x-4 items-center",
								"[&_dd]:text-content-primary [&_dd]:font-mono [&_dd]:leading-[22px] [&_dt]:font-medium",
							])}
						>
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
								<ProvisionerTags>
									{Object.entries(job.tags).map(([key, value]) => (
										<ProvisionerTag key={key} label={key} value={value} />
									))}
								</ProvisionerTags>
							</dd>
						</dl>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
