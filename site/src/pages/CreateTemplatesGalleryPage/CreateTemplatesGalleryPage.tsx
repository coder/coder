import { templateExamples } from "api/queries/templates";
import type { TemplateExample } from "api/typesGenerated";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { pageTitle } from "utils/page";
import { getTemplatesByTag } from "utils/starterTemplates";
import { CreateTemplatesPageView } from "./CreateTemplatesPageView";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";

const CreateTemplatesGalleryPage: FC = () => {
	const { showOrganizations } = useDashboard();
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
			{showOrganizations ? (
				<CreateTemplatesPageView
					error={templateExamplesQuery.error}
					starterTemplatesByTag={starterTemplatesByTag}
				/>
			) : (
				<StarterTemplatesPageView
					error={templateExamplesQuery.error}
					starterTemplatesByTag={starterTemplatesByTag}
				/>
			)}
		</>
	);
};

const removeScratchExample = (data: TemplateExample[]) => {
	return data.filter((example) => example.id !== "scratch");
};

export default CreateTemplatesGalleryPage;
