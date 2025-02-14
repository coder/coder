import { provisionerDaemons } from "api/queries/organizations";
import type { Organization, ProvisionerDaemon } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
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
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { relativeTime } from "utils/time";
import { DataGrid, DataGridSpace } from "./DataGrid";
import { DaemonJobStatusIndicator } from "./JobStatusIndicator";
import { Tag, Tags, TruncateTags } from "./Tags";

type ProvisionerDaemonsPageProps = {
	orgId: string;
};

export const ProvisionerDaemonsPage: FC<ProvisionerDaemonsPageProps> = ({
	orgId,
}) => {
	const {
		data: daemons,
		isLoadingError,
		refetch,
	} = useQuery({
		...provisionerDaemons(orgId),
		select: (data) =>
			data.toSorted((a, b) => {
				if (!a.last_seen_at && !b.last_seen_at) return 0;
				if (!a.last_seen_at) return 1;
				if (!b.last_seen_at) return -1;
				return (
					new Date(b.last_seen_at).getTime() -
					new Date(a.last_seen_at).getTime()
				);
			}),
	});

	return (
		<section className="flex flex-col gap-8">
			<h2 className="sr-only">Provisioner daemons</h2>
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
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState message="No provisioner daemons found" />
								</TableCell>
							</TableRow>
						)
					) : isLoadingError ? (
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState
									message="Error loading the provisioner daemons"
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
						<span className="sr-only">({isOpen ? "Hide" : "Show more"})</span>
						<span className="[&:first-letter]:uppercase">
							{relativeTime(
								new Date(daemon.last_seen_at ?? new Date().toISOString()),
							)}
						</span>
					</button>
				</TableCell>
				<TableCell>
					<span className="block whitespace-nowrap text-ellipsis overflow-hidden">
						{daemon.name}
					</span>
				</TableCell>
				<TableCell>
					{daemon.current_job ? (
						<div className="flex items-center gap-1 whitespace-nowrap">
							<Avatar
								variant="icon"
								src={daemon.current_job.template_icon}
								fallback={
									daemon.current_job.template_display_name ||
									daemon.current_job.template_name
								}
							/>
							{daemon.current_job.template_display_name ??
								daemon.current_job.template_name}
						</div>
					) : (
						<span className="whitespace-nowrap">Not linked</span>
					)}
				</TableCell>
				<TableCell>
					<TruncateTags tags={daemon.tags} />
				</TableCell>
				<TableCell>
					<StatusIndicator size="sm" variant={statusIndicatorVariant(daemon)}>
						<StatusIndicatorDot />
						<span className="[&:first-letter]:uppercase">
							{statusLabel(daemon)}
						</span>
					</StatusIndicator>
				</TableCell>
			</TableRow>

			{isOpen && (
				<TableRow>
					<TableCell colSpan={999} className="p-4 border-t-0">
						<DataGrid>
							<dt>Last seen:</dt>
							<dd>{daemon.last_seen_at}</dd>

							<dt>Creation time:</dt>
							<dd>{daemon.created_at}</dd>

							<dt>Version:</dt>
							<dd>{daemon.version}</dd>

							<dt>Tags:</dt>
							<dd>
								<Tags>
									{Object.entries(daemon.tags).map(([key, value]) => (
										<Tag key={key} label={key} value={value} />
									))}
								</Tags>
							</dd>

							{daemon.current_job && (
								<>
									<DataGridSpace />

									<dt>Last job:</dt>
									<dd>{daemon.current_job.id}</dd>

									<dt>Last job state:</dt>
									<dd>
										<DaemonJobStatusIndicator job={daemon.current_job} />
									</dd>
								</>
							)}

							{daemon.previous_job && (
								<>
									<DataGridSpace />

									<dt>Previous job:</dt>
									<dd>{daemon.previous_job.id}</dd>

									<dt>Previous job state:</dt>
									<dd>
										<DaemonJobStatusIndicator job={daemon.previous_job} />
									</dd>
								</>
							)}
						</DataGrid>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};

function statusIndicatorVariant(
	daemon: ProvisionerDaemon,
): StatusIndicatorProps["variant"] {
	if (daemon.previous_job && daemon.previous_job.status === "failed") {
		return "failed";
	}

	switch (daemon.status) {
		case "idle":
			return "success";
		case "busy":
			return "pending";
		default:
			return "inactive";
	}
}

function statusLabel(daemon: ProvisionerDaemon) {
	if (daemon.previous_job && daemon.previous_job.status === "failed") {
		return "Last job failed";
	}

	switch (daemon.status) {
		case "idle":
			return "Idle";
		case "busy":
			return "Busy...";
		case "offline":
			return "Disconnected";
		default:
			return "Unknown";
	}
}
