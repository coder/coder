import type {
	Template,
	TemplateVersion,
	WorkspaceResource,
} from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { TemplateResourcesTable } from "modules/templates/TemplateResourcesTable/TemplateResourcesTable";
import { type FC, useEffect } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { TemplateStats } from "./TemplateStats";

export interface TemplateSummaryPageViewProps {
	resources?: WorkspaceResource[];
	template: Template;
	activeVersion: TemplateVersion;
}

export const TemplateSummaryPageView: FC<TemplateSummaryPageViewProps> = ({
	resources,
	template,
	activeVersion,
}) => {
	const navigate = useNavigate();
	const location = useLocation();

	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useEffect(() => {
		if (location.hash === "#readme") {
			// We moved the readme to the docs page, but we known that some users
			// have bookmarked the readme or linked it elsewhere. Redirect them to the docs page.
			navigate("docs", { replace: true });
		}
	}, [template, navigate, location]);

	if (!resources) {
		return <Loader />;
	}

	const getStartedResources = (resources: WorkspaceResource[]) => {
		return resources.filter(
			(resource) => resource.workspace_transition === "start",
		);
	};

	return (
		<Stack spacing={4}>
			<TemplateStats template={template} activeVersion={activeVersion} />
			<TemplateResourcesTable resources={getStartedResources(resources)} />
		</Stack>
	);
};
