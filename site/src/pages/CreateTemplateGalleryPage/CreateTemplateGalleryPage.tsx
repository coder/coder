import { templateExamples } from "api/queries/templates";
import type { TemplateExample } from "api/typesGenerated";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { getTemplatesByTag } from "utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const CreateTemplatesGalleryPage: FC = () => {
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplatesByTag = templateExamplesQuery.data
		? // Currently, the scratch template should not be displayed on the starter templates page.
			getTemplatesByTag(removeScratchExample(templateExamplesQuery.data))
		: undefined;

	return (
		<>
			<Helmet>
				<title>{pageTitle("Create a Template")}</title>
			</Helmet>
			<CreateTemplateGalleryPageView
				error={templateExamplesQuery.error}
				starterTemplatesByTag={starterTemplatesByTag}
			/>
		</>
	);
};

const removeScratchExample = (data: TemplateExample[]) => {
	return data.filter((example) => example.id !== "scratch");
};

export default CreateTemplatesGalleryPage;
