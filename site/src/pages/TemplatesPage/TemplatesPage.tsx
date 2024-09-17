import { templateExamples, templates } from "api/queries/templates";
import { useFilter } from "components/Filter/Filter";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { TemplatesPageView } from "./TemplatesPageView";

export const TemplatesPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { showOrganizations } = useDashboard();

	const searchParamsResult = useSearchParams();
	const filter = useFilter({
		fallbackFilter: "deprecated:false",
		searchParamsResult,
		onUpdate: () => {}, // reset pagination
	});

	const templatesQuery = useQuery(templates({ q: filter.query }));
	const examplesQuery = useQuery({
		...templateExamples(),
		enabled: permissions.createTemplates,
	});
	const error = templatesQuery.error || examplesQuery.error;

	return (
		<>
			<Helmet>
				<title>{pageTitle("Templates")}</title>
			</Helmet>
			<TemplatesPageView
				error={error}
				filter={filter}
				showOrganizations={showOrganizations}
				canCreateTemplates={permissions.createTemplates}
				examples={examplesQuery.data}
				templates={templatesQuery.data}
			/>
		</>
	);
};

export default TemplatesPage;
