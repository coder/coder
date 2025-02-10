import { buildInfo } from "api/queries/buildInfo";
import {
	provisionerDaemonGroups,
	provisionerJobs,
} from "api/queries/organizations";
import type {
	Organization,
	ProvisionerJob,
	ProvisionerJobStatus,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
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
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import {
	BanIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	Tangent,
	TriangleAlertIcon,
} from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { useState, type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { relativeTime } from "utils/time";

const OrganizationProvisionersPage: FC = () => {
	// const { organization: organizationName } = useParams() as {
	// 	organization: string;
	// };
	const { organization } = useOrganizationSettings();
	const tab = useSearchParamsKey({
		key: "tab",
		defaultValue: "jobs",
	});
	// const { entitlements } = useDashboard();
	// const { metadata } = useEmbeddedMetadata();
	// const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	// const provisionersQuery = useQuery(provisionerDaemonGroups(organizationName));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	return (
		<>
			<Helmet>
				<title>
					{pageTitle(
						"Provisioners",
						organization.display_name || organization.name,
					)}
				</title>
			</Helmet>

			<div className="flex flex-col gap-12">
				<header className="flex flex-row items-baseline justify-between">
					<div className="flex flex-col gap-2">
						<h1 className="text-3xl m-0">Provisioners</h1>
					</div>
				</header>

				<main>
					<Tabs active={tab.value}>
						<TabsList>
							<TabLink value="jobs" to="?tab=jobs">
								Jobs
							</TabLink>
							<TabLink value="daemons" to="?tab=daemons">
								Daemons
							</TabLink>
						</TabsList>
					</Tabs>

					<div className="mt-6">
						{tab.value === "jobs" && <JobsTabContent org={organization} />}
					</div>
				</main>
			</div>
		</>
	);
};

type JobsTabContentProps = {
	org: Organization;
};

const JobsTabContent: FC<JobsTabContentProps> = ({ org }) => {
	const { data: jobs, isLoadingError } = useQuery(provisionerJobs(org.id));

	return (
		<section className="flex flex-col gap-8">
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
					</TableRow>
				</TableHeader>
				<TableBody>
					{jobs ? (
						jobs.length > 0 ? (
							jobs.map((j) => <JobRow key={j.id} job={j} />)
						) : (
							<TableEmpty message="No provisioner jobs found" />
						)
					) : isLoadingError ? (
						<TableEmpty message="Error loading the provisioner jobs" />
					) : (
						<TableLoader />
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
	const canCancel = ["pending", "running"].includes(job.status);
	const [isOpen, setIsOpen] = useState(false);

	return (
		<>
			<TableRow key={job.id}>
				<TableCell>
					<button
						className={cn([
							"flex items-center gap-1 p-0 bg-transparent border-0 text-inherit text-xs cursor-pointer",
							"transition-colors hover:text-content-primary font-medium",
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
							{relativeTime(new Date(job.created_at))}
						</span>
					</button>
				</TableCell>
				<TableCell>
					<Badge size="sm">{job.type}</Badge>
				</TableCell>
				<TableCell>
					{job.metadata.template_name ? (
						<div className="flex items-center gap-1">
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
						"Not linked to any template"
					)}
				</TableCell>
				<TableCell>
					<Badge size="sm">[foo=bar]</Badge>
				</TableCell>
				<TableCell>
					<StatusIndicator
						size="sm"
						variant={statusIndicatorVariant(job.status)}
					>
						<StatusIndicatorDot />
						<span className="[&:first-letter]:uppercase">{job.status}</span>
						{job.status === "failed" && (
							<TriangleAlertIcon className="size-icon-xs p-[1px]" />
						)}
						{job.status === "pending" &&
							`(${job.queue_position}/${job.queue_size})`}
					</StatusIndicator>
				</TableCell>
				<TableCell className="text-right">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									disabled={!canCancel}
									aria-label="Cancel job"
									size="icon"
									variant="outline"
								>
									<BanIcon />
								</Button>
							</TooltipTrigger>
							<TooltipContent>Cancel job</TooltipContent>
						</Tooltip>
					</TooltipProvider>
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
						<div
							className={cn([
								"grid grid-cols-[auto_1fr] gap-x-4 items-center",
								"[&_span:nth-child(even)]:text-content-primary [&_span:nth-child(even)]:font-mono",
								"[&_span:nth-child(even)]:leading-[22px]",
							])}
						>
							<span>Job ID:</span>
							<span>{job.id}</span>

							<span>Available provisioners:</span>
							<span>
								{job.available_workers
									? JSON.stringify(job.available_workers)
									: "[]"}
							</span>

							<span>Completed by provisioner:</span>
							<span>{job.worker_id}</span>

							<span>Associated workspace:</span>
							<span>{job.metadata.workspace_name ?? "null"}</span>

							<span>Creation time:</span>
							<span>{job.created_at}</span>

							<span>Queue:</span>
							<span>
								{job.queue_position}/{job.queue_size}
							</span>
						</div>
					</TableCell>
				</TableRow>
			)}
		</>
	);
};

function statusIndicatorVariant(
	status: ProvisionerJobStatus,
): StatusIndicatorProps["variant"] {
	switch (status) {
		case "succeeded":
			return "success";
		case "failed":
			return "failed";
		case "pending":
		case "running":
		case "canceling":
			return "pending";
		case "canceled":
		case "unknown":
			return "inactive";
	}
}

export default OrganizationProvisionersPage;
