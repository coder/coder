import type { FC } from "react";
import { useQuery } from "react-query";
import { deploymentConfig } from "#/api/queries/deployment";
import { templateExamples } from "#/api/queries/templates";
import { pageTitle } from "#/utils/page";
import { getTemplatesByTag } from "#/utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const CreateTemplatesGalleryPage: FC = () => {
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplatesByTag = templateExamplesQuery.data
		? getTemplatesByTag(templateExamplesQuery.data)
		: undefined;

	const { data: deploymentConfigData } = useQuery(deploymentConfig());
	const builderDisabled =
		deploymentConfigData?.config?.template_builder?.disabled ?? false;

	return (
		<>
			<title>{pageTitle("Create a Template")}</title>

			<CreateTemplateGalleryPageView
				error={templateExamplesQuery.error}
				starterTemplatesByTag={starterTemplatesByTag}
				templateBuilderEnabled={!builderDisabled}
			/>
		</>
	);
};

export default CreateTemplatesGalleryPage;
