import type { ComponentProps, FC } from "react";
import type { Organization, ProvisionerJob } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Link } from "#/components/Link/Link";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import { JobRow } from "./JobRow";
import { ProvisionerJobsFilter } from "./ProvisionerJobsFilter";

type OrganizationProvisionerJobsPageViewProps = {
	jobs: readonly ProvisionerJob[] | undefined;
	jobsQuery: PaginationResult;
	organization: Organization | undefined;
	error: unknown;
	isNonInitialPage: boolean;
	onRetry: () => void;
	filterProps: ComponentProps<typeof ProvisionerJobsFilter>;
};

const OrganizationProvisionerJobsPageView: FC<
	OrganizationProvisionerJobsPageViewProps
> = ({
	jobs,
	jobsQuery,
	organization,
	error,
	isNonInitialPage,
	onRetry,
	filterProps,
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
	const openJobIds = filterProps.filter.values.id ?? "";

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<title>
				{pageTitle(
					"Provisioner Jobs",
					organization.display_name || organization.name,
				)}
			</title>

			<section className="flex flex-col gap-4">
				<SettingsHeader>
					<SettingsHeaderTitle>Provisioner Jobs</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Provisioner Jobs are the individual tasks assigned to Provisioners
						when the workspaces are being built.{" "}
						<Link href={docs("/admin/provisioners")}>View docs</Link>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<ProvisionerJobsFilter {...filterProps} />

				<PaginationContainer query={jobsQuery} paginationUnitLabel="jobs">
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
										defaultIsOpen={openJobIds.includes(j.id)}
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
