import { provisionerDaemons } from "api/queries/organizations";
import type { Organization, ProvisionerDaemon } from "api/typesGenerated";
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
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { useState, type FC } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { relativeTime } from "utils/time";
import { JobStatusIndicator } from "./JobStatusIndicator";
import { Avatar } from "components/Avatar/Avatar";
import { DataGrid, DataGridSpace } from "./DataGrid";
import { ShrinkTags, Tag, Tags } from "./Tags";
import { Loader } from "components/Loader/Loader";
import { EmptyState } from "components/EmptyState/EmptyState";

type ProvisionerDaemonsPageProps = {
	org: Organization;
};

export const ProvisionerDaemonsPage: FC<ProvisionerDaemonsPageProps> = ({
	org,
}) => {
	const { data: daemons, isLoadingError } = useQuery({
		...provisionerDaemons(org.id),
		select: (data) =>
			data.toSorted((a, b) => {
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
								<EmptyState message="Error loading the provisioner daemons" />
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
					<ShrinkTags tags={daemon.tags} />
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
							<span>Last seen:</span>
							<span>{daemon.last_seen_at}</span>

							<span>Creation time:</span>
							<span>{daemon.created_at}</span>

							<span>Version:</span>
							<span>{daemon.version}</span>

							<span>Tags:</span>
							<span>
								<Tags>
									{Object.entries(daemon.tags).map(([key, value]) => (
										<Tag key={key} label={key} value={value} />
									))}
								</Tags>
							</span>

							{daemon.current_job && (
								<>
									<DataGridSpace />

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
									<DataGridSpace />

									<span>Previous job:</span>
									<span>{daemon.previous_job.id}</span>

									<span>Previous job state:</span>
									<span>
										<JobStatusIndicator job={daemon.previous_job} />
									</span>
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
		case "offline":
		case null:
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
		case null:
			return "Unknown";
	}
}
