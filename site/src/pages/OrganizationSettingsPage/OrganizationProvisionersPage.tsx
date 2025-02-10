import { buildInfo } from "api/queries/buildInfo";
import {
	provisionerDaemonGroups,
	provisionerJobs,
} from "api/queries/organizations";
import type { Organization, ProvisionerJobStatus } from "api/typesGenerated";
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
import { BanIcon, TriangleAlertIcon } from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { relativeTime } from "utils/time";

const OrganizationProvisionersPage: FC = () => {
	// const { organization: organizationName } = useParams() as {
	// 	organization: string;
	// };
	const { organization } = useOrganizationSettings();
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
					<Tabs active="jobs">
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
						<JobsTabContent org={organization} />
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
	const { organization } = useOrganizationSettings();
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
							jobs.map(({ metadata, ...job }) => {
								if (!metadata) {
									throw new Error(
										`Metadata is required but it is missing in the job ${job.id}`,
									);
								}

								const canCancel = ["pending", "running"].includes(job.status);

								return (
									<TableRow key={job.id}>
										<TableCell className="[&:first-letter]:uppercase">
											{relativeTime(new Date(job.created_at))}
										</TableCell>
										<TableCell>
											<Badge size="sm">{job.type}</Badge>
										</TableCell>
										<TableCell>
											<div className="flex items-center gap-1">
												<Avatar
													variant="icon"
													src={metadata.template_icon}
													fallback={
														metadata.template_display_name ||
														metadata.template_name
													}
												/>
												{metadata.template_display_name ??
													metadata.template_name}
											</div>
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
												<span className="[&:first-letter]:uppercase">
													{job.status}
												</span>
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
								);
							})
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
