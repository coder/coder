import type { FC } from "react";
import { useQuery } from "react-query";
import { deploymentConfig } from "#/api/queries/deployment";
import { templateExamples } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { getTemplatesByTag } from "#/utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const CreateTemplatesGalleryPage: FC = () => {
	const { permissions } = useAuthenticated();
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplatesByTag = templateExamplesQuery.data
		? getTemplatesByTag(templateExamplesQuery.data)
		: undefined;
	const deploymentConfigQuery = useQuery({
		...deploymentConfig(),
		enabled: permissions.createTemplates,
	});
	const templateBuilderEnabled =
		deploymentConfigQuery.isSuccess &&
		!deploymentConfigQuery.data?.config?.template_builder?.disabled;

	return (
		<>
			<title>{pageTitle("Create a Template")}</title>

			<CreateTemplateGalleryPageView
				error={templateExamplesQuery.error}
				starterTemplatesByTag={starterTemplatesByTag}
				templateBuilderEnabled={templateBuilderEnabled}
			/>
		</>
	);
};

export default CreateTemplatesGalleryPage;
