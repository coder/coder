import { XIcon } from "lucide-react";
import type { FC } from "react";
import type {
	Organization,
	ProvisionerJob,
	ProvisionerJobStatus,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	StatusIndicator,
	StatusIndicatorDot,
	type StatusIndicatorProps,
} from "#/components/StatusIndicator/StatusIndicator";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import { JobRow } from "./JobRow";

const variantByStatus: Record<
	ProvisionerJobStatus,
	StatusIndicatorProps["variant"]
> = {
	succeeded: "success",
	failed: "failed",
	pending: "pending",
	running: "pending",
	canceling: "pending",
	canceled: "inactive",
	unknown: "inactive",
};

const StatusFilters: ProvisionerJobStatus[] = [
	"succeeded",
	"pending",
	"running",
	"canceling",
	"canceled",
	"failed",
	"unknown",
];

type JobProvisionersFilter = {
	status: string;
	ids: string;
};

type OrganizationProvisionerJobsPageViewProps = {
	jobs: readonly ProvisionerJob[] | undefined;
	jobsQuery: PaginationResult;
	organization: Organization | undefined;
	error: unknown;
	filter: JobProvisionersFilter;
	isNonInitialPage: boolean;
	onRetry: () => void;
	onFilterChange: (filter: JobProvisionersFilter) => void;
};

const OrganizationProvisionerJobsPageView: FC<
	OrganizationProvisionerJobsPageViewProps
> = ({
	jobs,
	jobsQuery,
	organization,
	error,
	filter,
	isNonInitialPage,
	onFilterChange,
	onRetry,
}) => {
	if (!organization) {
		return (
			<>
				<title>{pageTitle("Provisioner Jobs")}</title>

				<EmptyState message="Organization not found" />
			</>
		);
	}

	const isLoading =
		(jobs === undefined || jobsQuery.totalRecords === undefined) && !error;
	const isEmpty = !isLoading && jobs?.length === 0;

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<title>
				{pageTitle(
					"Provisioner Jobs",
					organization.display_name || organization.name,
				)}
			</title>

			<section>
				<SettingsHeader>
					<SettingsHeaderTitle>Provisioner Jobs</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Provisioner Jobs are the individual tasks assigned to Provisioners
						when the workspaces are being built.{" "}
						<Link href={docs("/admin/provisioners")}>View docs</Link>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<div className="flex items-center gap-2">
					{filter.ids && (
						<div className="relative">
							<Badge className="h-10 text-sm pl-3 pr-10 font-mono">
								{filter.ids}
							</Badge>
							<div className="size-10 flex items-center justify-center absolute top-0 right-0">
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											size="icon"
											variant="subtle"
											onClick={() => {
												onFilterChange({ ...filter, ids: "" });
											}}
										>
											<span className="sr-only">Clear ID</span>
											<XIcon />
										</Button>
									</TooltipTrigger>
									<TooltipContent>Clear ID</TooltipContent>
								</Tooltip>
							</div>
						</div>
					)}

					<Select
						value={filter.status}
						onValueChange={(status) => {
							onFilterChange({
								...filter,
								status,
							});
						}}
					>
						<SelectTrigger className="w-[180px]" data-testid="status-filter">
							<SelectValue placeholder="All statuses" />
						</SelectTrigger>
						<SelectContent>
							<SelectGroup>
								{StatusFilters.map((status) => (
									<SelectItem key={status} value={status}>
										<StatusIndicator variant={variantByStatus[status]}>
											<StatusIndicatorDot />
											<span className="block first-letter:uppercase">
												{status}
											</span>
										</StatusIndicator>
									</SelectItem>
								))}
							</SelectGroup>
						</SelectContent>
					</Select>
				</div>

				<PaginationContainer
					query={jobsQuery}
					paginationUnitLabel="jobs"
					className="mt-6"
				>
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
							{error ? (
								<TableEmpty
									message="Error loading the provisioner jobs"
									cta={
										<Button size="sm" onClick={onRetry}>
											Retry
										</Button>
									}
								/>
							) : isLoading ? (
								<TableLoader />
							) : isEmpty ? (
								<TableEmpty
									message={
										isNonInitialPage
											? "No provisioner jobs available on this page"
											: "No provisioner jobs found"
									}
								/>
							) : (
								jobs?.map((j) => (
									<JobRow
										defaultIsOpen={filter.ids.includes(j.id)}
										key={j.id}
										job={j}
									/>
								))
							)}
						</TableBody>
					</Table>
				</PaginationContainer>
			</section>
		</div>
	);
};

export default OrganizationProvisionerJobsPageView;
