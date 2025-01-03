import { templateExamples } from "api/queries/templates";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router";
import { pageTitle } from "utils/page";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const StarterTemplatePage: FC = () => {
	const { exampleId } = useParams() as { exampleId: string };
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplate = templateExamplesQuery.data?.find(
		(example) => example.id === exampleId,
	);

	return (
		<>
			<title>{pageTitle(starterTemplate?.name ?? exampleId)}</title>

			<StarterTemplatePageView
				starterTemplate={starterTemplate}
				error={templateExamplesQuery.error}
			/>
		</>
	);
};

export default StarterTemplatePage;
