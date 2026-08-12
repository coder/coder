import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import { templateExamples } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const StarterTemplatePage: FC = () => {
	const { exampleId } = useParams() as { exampleId: string };
	const { permissions } = useAuthenticated();
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplate = templateExamplesQuery.data?.find(
		(example) => example.id === exampleId,
	);
	const deploymentConfigQuery = useQuery({
		...deploymentConfig(),
		enabled: permissions.createTemplates,
	});
	const templateBuilderEnabled =
		deploymentConfigQuery.isSuccess &&
		!deploymentConfigQuery.data?.config?.template_builder?.disabled;

	return (
		<>
			<title>{pageTitle(starterTemplate?.name ?? exampleId)}</title>

			<StarterTemplatePageView
				starterTemplate={starterTemplate}
				templateBuilderEnabled={templateBuilderEnabled}
				error={templateExamplesQuery.error}
			/>
		</>
	);
};

export default StarterTemplatePage;
