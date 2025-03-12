import type { API } from "api/api";
import { provisionerJobs } from "api/queries/organizations";
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
	type OrganizationSettingsValue,
	useOrganizationSettings,
} from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { JobRow } from "./JobRow";

type OrganizationProvisionerJobsPageProps = {
	getProvisionerJobs?: typeof API.getProvisionerJobs;
};

const OrganizationProvisionerJobsPage: FC<
	OrganizationProvisionerJobsPageProps
> = ({ getProvisionerJobs }) => {
	const { organization } = useOrganizationSettings();

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const {
		data: jobs,
		isLoadingError,
		refetch,
	} = useQuery(provisionerJobs(organization.id, getProvisionerJobs));

	return (
		<>
			<Helmet>
				<title>
					{pageTitle(
						"Provisioner Jobs",
						organization.display_name || organization.name,
					)}
				</title>
			</Helmet>

			<section className="flex flex-col gap-8">
				<header className="flex flex-row items-baseline justify-between">
					<div className="flex flex-col gap-2">
						<h1 className="text-3xl m-0">Provisioner Jobs</h1>
						<p className="text-sm text-content-secondary m-0">
							Provisioner Jobs are the individual tasks assigned to Provisioners
							when the workspaces are being built.{" "}
							<Link href={docs("/admin/provisioners")}>View docs</Link>
						</p>
					</div>
				</header>

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
										cta={
											<Button size="sm" onClick={() => refetch()}>
												Retry
											</Button>
										}
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
		</>
	);
};

export default OrganizationProvisionerJobsPage;
