import { templateExamples } from "api/queries/templates";
import type { FC } from "react";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { getTemplatesByTag } from "utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const CreateTemplatesGalleryPage: FC = () => {
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplatesByTag = templateExamplesQuery.data
		? getTemplatesByTag(templateExamplesQuery.data)
		: undefined;

	return (
		<>
			<title>{pageTitle("Create a Template")}</title>

			<CreateTemplateGalleryPageView
				error={templateExamplesQuery.error}
				starterTemplatesByTag={starterTemplatesByTag}
			/>
		</>
	);
};

export default CreateTemplatesGalleryPage;
