import { useQuery } from "react-query";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/Breadcrumb/Breadcrumb";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { getOrganizationLabel } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { useSearchParamsKey } from "#/hooks/useSearchParamsKey";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { GitHubRunnerPools } from "./GitHubRunnerPools";

const GitHubRunnersPage: React.FC = () => {
	const { organizations } = useDashboard();
	const permissionsQuery = useQuery(
		organizationsPermissions(organizations.map((org) => org.id)),
	);
	const selectedOrganization = useSearchParamsKey({ key: "organization" });
	const availableOrganizations = organizations.filter(
		(org) => permissionsQuery.data?.[org.id]?.viewGitHubRunnerPools,
	);
	const organization =
		availableOrganizations.find(
			(org) => org.id === selectedOrganization.value,
		) ?? availableOrganizations[0];
	const permissions = organization && permissionsQuery.data?.[organization.id];

	return (
		<>
			<title>{pageTitle("GitHub runners")}</title>
			<Breadcrumb>
				<BreadcrumbList>
					<BreadcrumbItem>
						<span>Admin Settings</span>
					</BreadcrumbItem>
					<BreadcrumbSeparator />
					<BreadcrumbItem>
						<BreadcrumbPage>GitHub runners</BreadcrumbPage>
					</BreadcrumbItem>
				</BreadcrumbList>
			</Breadcrumb>
			<div className="border-0 border-b border-solid border-border" />
			<Margins className="pb-12">
				{permissionsQuery.error && (
					<div className="mt-6 space-y-4">
						<ErrorAlert error={permissionsQuery.error} />
						<Button
							variant="outline"
							onClick={() => void permissionsQuery.refetch()}
						>
							Retry permissions
						</Button>
					</div>
				)}
				{!permissionsQuery.data && !permissionsQuery.error ? (
					<Loader />
				) : organization && permissions ? (
					<>
						{availableOrganizations.length > 1 && (
							<div className="mt-6 max-w-xs">
								<Select
									value={organization.id}
									onValueChange={selectedOrganization.setValue}
								>
									<SelectTrigger aria-label="Organization">
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										{availableOrganizations.map((org) => (
											<SelectItem key={org.id} value={org.id}>
												{getOrganizationLabel(org, availableOrganizations)}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
						)}
						<GitHubRunnerPools
							key={organization.id}
							organization={organization}
							permissions={permissions}
						/>
					</>
				) : (
					!permissionsQuery.error && (
						<p className="py-12 text-content-secondary">
							You do not have permission to view GitHub runner pools in any
							organization.
						</p>
					)
				)}
			</Margins>
		</>
	);
};

export default GitHubRunnersPage;
