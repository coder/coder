import { useQuery } from "react-query";
import { templateBuilderBases } from "#/api/queries/templateBuilder";
import { templateExamples } from "#/api/queries/templates";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { getTemplatesByTag } from "#/utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const CreateTemplatesGalleryPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const templateExamplesQuery = useQuery({
		...templateExamples(),
		enabled: permissions.createTemplates,
	});
	const starterTemplatesByTag = templateExamplesQuery.data
		? getTemplatesByTag(templateExamplesQuery.data)
		: undefined;

	const basesQuery = useQuery({
		...templateBuilderBases(),
		enabled: permissions.createTemplates,
		retry: false,
	});

	return (
		<RequirePermission isFeatureVisible={permissions.createTemplates}>
			<title>{pageTitle("Create a Template")}</title>

			<CreateTemplateGalleryPageView
				error={templateExamplesQuery.error}
				starterTemplatesByTag={starterTemplatesByTag}
				templateBuilderEnabled={basesQuery.isSuccess}
			/>
		</RequirePermission>
	);
};

export default CreateTemplatesGalleryPage;
