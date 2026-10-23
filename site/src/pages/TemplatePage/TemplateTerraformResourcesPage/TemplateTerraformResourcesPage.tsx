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
import { TemplateTerraformResourcesPageView } from "./TemplateTerraformResourcesPageView";

const TemplateTerraformResourcesPage: React.FC = () => {
	const { template, permissions } = useTemplateLayoutContext();
	return (
		<TemplateResourceManagement
			template={template}
			canUpdateTemplate={permissions.canUpdateTemplate}
		/>
	);
};

type TemplateResourceManagementProps = {
	template: Template;
	canUpdateTemplate: boolean;
};

/** Manages template revisions and runs independently of workspace versions. */
export const TemplateResourceManagement: React.FC<
	TemplateResourceManagementProps
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
						? "Enable the template-resources experiment to manage template resources."
						: "Template update permission is required to view template resources."
				}
			/>
		);
	}

	return (
		<>
			<title>{getTemplatePageTitle("Template resources", template)}</title>
			<TemplateTerraformResourcesPageView
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

export default TemplateTerraformResourcesPage;
