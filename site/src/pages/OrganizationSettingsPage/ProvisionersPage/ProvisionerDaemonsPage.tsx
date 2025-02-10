import { provisionerDaemons } from "api/queries/organizations";
import type {
	Organization,
	ProvisionerDaemon,
	ProvisionerDaemonStatus,
} from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Link } from "components/Link/Link";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "components/StatusIndicator/StatusIndicator";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { useState, type FC } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { relativeTime } from "utils/time";
import { JobStatusIndicator } from "./JobStatusIndicator";

type ProvisionerDaemonsPageProps = {
	org: Organization;
};

export const ProvisionerDaemonsPage: FC<ProvisionerDaemonsPageProps> = ({
	org,
}) => {
	const { data: daemons, isLoadingError } = useQuery(
		provisionerDaemons(org.id),
	);

	return (
		<section className="flex flex-col gap-8">
			<p className="text-sm text-content-secondary m-0 mt-2">
				Coder server runs provisioner daemons which execute terraform during
				workspace and template builds.{" "}
				<Link
					href={docs(
						"/tutorials/best-practices/security-best-practices#provisioner-daemons",
					)}
				>
					View docs
				</Link>
			</p>

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Last seen</TableHead>
						<TableHead>Name</TableHead>
						<TableHead>Template</TableHead>
						<TableHead>Tags</TableHead>
						<TableHead>Status</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{daemons ? (
						daemons.length > 0 ? (
							daemons.map((d) => <DaemonRow key={d.id} daemon={d} />)
						) : (
							<TableEmpty message="No provisioner daemons found" />
						)
					) : isLoadingError ? (
						<TableEmpty message="Error loading the provisioner daemons" />
					) : (
						<TableLoader />
					)}
				</TableBody>
			</Table>
		</section>
	);
};

type DaemonRowProps = {
	daemon: ProvisionerDaemon;
};

const DaemonRow: FC<DaemonRowProps> = ({ daemon }) => {
	const [isOpen, setIsOpen] = useState(false);

	return (
		<>
			<TableRow key={daemon.id}>
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
						<span className="[&:first-letter]:uppercase">
							{relativeTime(
								new Date(daemon.last_seen_at ?? new Date().toISOString()),
							)}
						</span>
					</button>
				</TableCell>
				<TableCell>{daemon.name}</TableCell>
				<TableCell>Template</TableCell>
				<TableCell>
					<div className="flex items-center gap-1 flex-wrap">
						{Object.entries(daemon.tags).map(([k, v]) => (
							<Badge size="sm" key={k} className="whitespace-nowrap">
								[{k}
								{v && `=${v}`}]
							</Badge>
						))}
					</div>
				</TableCell>
				<TableCell>
					<StatusIndicator
						size="sm"
						variant={statusIndicatorVariant(daemon.status)}
					>
						<StatusIndicatorDot />
						<span className="[&:first-letter]:uppercase">{daemon.status}</span>
					</StatusIndicator>
				</TableCell>
			</TableRow>

			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						<div
							className={cn([
								"grid grid-cols-[auto_1fr] gap-x-4 items-center",
								"[&_span:nth-child(even)]:text-content-primary [&_span:nth-child(even)]:font-mono",
								"[&_span:nth-child(even)]:leading-[22px]",
							])}
						>
							<span>Last seen:</span>
							<span>{daemon.last_seen_at}</span>

							<span>Creation time:</span>
							<span>{daemon.created_at}</span>

							<span>Version:</span>
							<span>{daemon.version}</span>

							{daemon.current_job && (
								<>
									<span>Last job:</span>
									<span>{daemon.current_job.id}</span>

									<span>Last job state:</span>
									<span>
										<JobStatusIndicator job={daemon.current_job} />
									</span>
								</>
							)}

							{daemon.previous_job && (
								<>
									<span>Previous job:</span>
									<span>{daemon.previous_job.id}</span>

									<span>Previous job state:</span>
									<span>
										<JobStatusIndicator job={daemon.previous_job} />
									</span>
								</>
							)}
						</div>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};

function statusIndicatorVariant(
	status: ProvisionerDaemonStatus | null,
): StatusIndicatorProps["variant"] {
	switch (status) {
		case "idle":
			return "success";
		case "busy":
			return "pending";
		case "offline":
		case null:
			return "inactive";
	}
}
