import { EmptyState } from "components/EmptyState/EmptyState";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { ProvisionerDaemonsPage } from "./ProvisionerDaemonsPage";
import { ProvisionerJobsPage } from "./ProvisionerJobsPage";

const ProvisionersPage: FC = () => {
	const { organization } = useOrganizationSettings();
	const tab = useSearchParamsKey({
		key: "tab",
		defaultValue: "jobs",
	});

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
						{tab.value === "jobs" && <ProvisionerJobsPage org={organization} />}
						{tab.value === "daemons" && (
							<ProvisionerDaemonsPage org={organization} />
						)}
					</div>
				</main>
			</div>
		</>
	);
};

export default ProvisionersPage;
