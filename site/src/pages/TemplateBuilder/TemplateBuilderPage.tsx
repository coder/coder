import type { FC } from "react";
import { useQuery } from "react-query";
import { Navigate } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";
import { TemplateBuilderPageView } from "./TemplateBuilderPageView";

const TemplateBuilderPage: FC = () => {
	const { data, error, isLoading } = useQuery(deploymentConfig());

	if (isLoading) {
		return <Loader />;
	}

	// if the template builder is disabled in the deployment config,
	// redirect to the new template page
	const builderDisabled = data?.config?.template_builder?.disabled ?? false;
	if (builderDisabled) {
		return <Navigate to="/templates/new" replace />;
	}

	return (
		<>
			<title>{pageTitle("Create Template")}</title>
			<TemplateBuilderPageView error={error} />
		</>
	);
};

export default TemplateBuilderPage;
