import type { Template, WorkspaceResource } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { TemplateResourcesTable } from "modules/templates/TemplateResourcesTable/TemplateResourcesTable";
import { type FC } from "react";
import { Navigate, useLocation } from "react-router-dom";

export interface TemplateResourcesPageViewProps {
	resources?: WorkspaceResource[];
	template: Template;
}

export const TemplateResourcesPageView: FC<TemplateResourcesPageViewProps> = ({
	resources,
}) => {
	const location = useLocation();

	if (location.hash === "#readme") {
		return <Navigate to="docs" replace />;
	}

	if (!resources) {
		return <Loader />;
	}

	const getStartedResources = (resources: WorkspaceResource[]) => {
		return resources.filter(
			(resource) => resource.workspace_transition === "start",
		);
	};

	return <TemplateResourcesTable resources={getStartedResources(resources)} />;
};
