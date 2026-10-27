import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	createTemplateResourceRun,
	templateResources,
} from "#/api/queries/templateResources";
import type { Template } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { useTemplateLayoutContext } from "#/pages/TemplatePage/TemplateLayout";
import { getTemplatePageTitle } from "../utils";
import { TemplateSharedResourcesPageView } from "./TemplateSharedResourcesPageView";

const TemplateSharedResourcesPage: React.FC = () => {
	const { template, permissions } = useTemplateLayoutContext();
	return (
		<SharedResourceManagement
			template={template}
			canUpdateTemplate={permissions.canUpdateTemplate}
		/>
	);
};

type SharedResourceManagementProps = {
	template: Template;
	canUpdateTemplate: boolean;
};

/** Manages shared resource revisions and runs independently of workspace versions. */
export const SharedResourceManagement: React.FC<
	SharedResourceManagementProps
> = ({ template, canUpdateTemplate }) => {
	const queryClient = useQueryClient();
	const getLink = useLinks();
	const { experiments } = useDashboard();
	const enabled = experiments.includes("template-resources");
	const query = useQuery({
		...templateResources(template.id),
		enabled: enabled && canUpdateTemplate,
	});
	const run = useMutation(createTemplateResourceRun(queryClient, template.id));

	if (!enabled || !canUpdateTemplate) {
		return (
			<ErrorAlert
				error={
					!enabled
						? "Enable the template-resources experiment to manage shared resources."
						: "Template update permission is required to view shared resources."
				}
			/>
		);
	}

	return (
		<>
			<title>{getTemplatePageTitle("Shared resources", template)}</title>
			<TemplateSharedResourcesPageView
				key={template.id}
				resources={query.data}
				error={query.error}
				isLoading={query.isLoading}
				onRetry={() => void query.refetch()}
				editFilesHref={`${getLink(linkToTemplate(template.organization_name, template.name))}/edit?scope=template`}
				isRunning={run.isPending}
				runError={run.error}
				onRun={(request, onSuccess) => run.mutate(request, { onSuccess })}
			/>
		</>
	);
};

export default TemplateSharedResourcesPage;
