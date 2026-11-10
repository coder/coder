import { useQuery } from "react-query";
import { useParams } from "react-router";
import { templateExamples } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const StarterTemplatePage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { exampleId } = useParams() as { exampleId: string };
	const templateExamplesQuery = useQuery(templateExamples());
	const starterTemplate = templateExamplesQuery.data?.find(
		(example) => example.id === exampleId,
	);

	return (
		<>
			<title>{pageTitle(starterTemplate?.name ?? exampleId)}</title>

			<StarterTemplatePageView
				canCreateTemplates={permissions.createTemplates}
				starterTemplate={starterTemplate}
				error={templateExamplesQuery.error}
			/>
		</>
	);
};

export default StarterTemplatePage;
