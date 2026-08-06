import type { FC } from "react";
import { useSearchParams } from "react-router";
import { paginatedProvisionerJobs } from "#/api/queries/organizations";
import { useFilter } from "#/components/Filter/Filter";
import { isNonInitialPage } from "#/components/PaginationWidget/utils";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import OrganizationProvisionerJobsPageView from "./OrganizationProvisionerJobsPageView";
import {
	useProvisionerJobStatusFilterMenu,
	useProvisionerJobTemplateFilterMenu,
	useProvisionerJobTypeFilterMenu,
} from "./ProvisionerJobsFilter";

const OrganizationProvisionerJobsPage: FC = () => {
	const { organization } = useOrganizationSettings();
	const [searchParams, setSearchParams] = useSearchParams();
	const jobsQuery = usePaginatedQuery(
		paginatedProvisionerJobs(organization?.id ?? "", searchParams),
	);

	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: jobsQuery.goToFirstPage,
	});

	const statusMenu = useProvisionerJobStatusFilterMenu({
		value: filter.values.status,
		onChange: (option) =>
			filter.update({
				...filter.values,
				status: option?.value,
			}),
	});

	const typeMenu = useProvisionerJobTypeFilterMenu({
		value: filter.values.type,
		onChange: (option) =>
			filter.update({
				...filter.values,
				type: option?.value,
			}),
	});

	const templateMenu = useProvisionerJobTemplateFilterMenu({
		organizationId: organization?.id,
		value: filter.values.template,
		onChange: (option) =>
			filter.update({
				...filter.values,
				template: option?.value,
			}),
	});

	return (
		<OrganizationProvisionerJobsPageView
			jobs={jobsQuery.data?.jobs}
			jobsQuery={jobsQuery}
			organization={organization}
			error={jobsQuery.error}
			isNonInitialPage={isNonInitialPage(searchParams)}
			onRetry={jobsQuery.refetch}
			filterProps={{
				filter,
				error: jobsQuery.error,
				menus: {
					status: statusMenu,
					type: typeMenu,
					template: templateMenu,
				},
			}}
		/>
	);
};

export default OrganizationProvisionerJobsPage;
