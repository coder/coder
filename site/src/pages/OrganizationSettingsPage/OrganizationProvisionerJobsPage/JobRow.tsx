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
import { JobStatusIndicator } from "./JobStatusIndicator";
import { Tag, Tags, TruncateTags } from "./Tags";

type JobRowProps = {
	job: ProvisionerJob;
	defaultOpen?: boolean;
};

export const JobRow: FC<JobRowProps> = ({ job, defaultOpen }) => {
	const metadata = job.metadata;
	const [isOpen, setIsOpen] = useState(defaultOpen);

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
								<Tags>
									{Object.entries(job.tags).map(([key, value]) => (
										<Tag key={key} label={key} value={value} />
									))}
								</Tags>
							</dd>
						</dl>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};
