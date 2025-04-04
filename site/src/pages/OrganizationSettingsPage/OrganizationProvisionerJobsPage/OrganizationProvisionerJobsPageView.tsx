import type { Organization, ProvisionerJob } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import { Loader } from "components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { JobRow } from "./JobRow";

type OrganizationProvisionerJobsPageViewProps = {
	jobs: ProvisionerJob[] | undefined;
	organization: Organization | undefined;
	error: unknown;
	onRetry: () => void;
};

const OrganizationProvisionerJobsPageView: FC<
	OrganizationProvisionerJobsPageViewProps
> = ({ jobs, organization, error, onRetry }) => {
	if (!organization) {
		return (
			<>
				<Helmet>
					<title>{pageTitle("Provisioner Jobs")}</title>
				</Helmet>
				<EmptyState message="Organization not found" />
			</>
		);
	}

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

			<section>
				<SettingsHeader>
					<SettingsHeaderTitle>Provisioner Jobs</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Provisioner Jobs are the individual tasks assigned to Provisioners
						when the workspaces are being built.{" "}
						<Link href={docs("/admin/provisioners")}>View docs</Link>
					</SettingsHeaderDescription>
				</SettingsHeader>

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
						) : error ? (
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message="Error loading the provisioner jobs"
										cta={
											<Button size="sm" onClick={onRetry}>
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

export default OrganizationProvisionerJobsPageView;
