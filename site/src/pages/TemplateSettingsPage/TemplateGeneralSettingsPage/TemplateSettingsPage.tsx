import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	invalidateTemplateListQueries,
	templateByNameKey,
} from "#/api/queries/templates";
import type { Template, UpdateTemplateMeta } from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateSettingsPageView } from "./TemplateSettingsPageView";

export const templateMetaWithoutExitNodeBindings = (
	template: Template,
): UpdateTemplateMeta => {
	const {
		exit_node_ids: _exitNodeIds,
		exit_node_enforce: _exitNodeEnforce,
		...templateMeta
	} = template;
	return templateMeta;
};

const TemplateSettingsPage: FC = () => {
	const { template: templateName } = useParams() as { template: string };
	const navigate = useNavigate();
	const getLink = useLinks();
	const { template } = useTemplateSettings();
	const queryClient = useQueryClient();
	const { entitlements } = useDashboard();
	const accessControlEnabled = entitlements.features.access_control.enabled;
	const advancedSchedulingEnabled =
		entitlements.features.advanced_template_scheduling.enabled;
	const sharedPortControlsEnabled =
		entitlements.features.control_shared_ports.enabled;

	const {
		mutate: updateTemplate,
		isPending: isSubmitting,
		error: submitError,
	} = useMutation({
		mutationFn: (data: UpdateTemplateMeta) => {
			return API.updateTemplateMeta(template.id, data);
		},
		onSuccess: async (data) => {
			// This update has a chance to return a 304 which means nothing was updated.
			// In this case, the return payload will be empty and we should use the
			// original template data.
			if (!data) {
				data = template;
			} else {
				// Use data.name because an admin may have renamed the template.
				await Promise.all([
					invalidateTemplateListQueries(queryClient),
					queryClient.invalidateQueries({
						queryKey: templateByNameKey(template.organization_name, data.name),
					}),
				]);
			}
			toast.success(`Template "${data.name}" updated successfully.`);
			navigate(getLink(linkToTemplate(data.organization_name, data.name)));
		},
		onError: (error) => {
			toast.error(
				getErrorMessage(error, `Failed to update template "${template.name}".`),
				{
					description: getErrorDetail(error),
				},
			);
		},
	});

	return (
		<>
			<title>{pageTitle(template.name, "General Settings")}</title>

			<TemplateSettingsPageView
				isSubmitting={isSubmitting}
				template={template}
				submitError={submitError}
				onCancel={() => {
					navigate(
						getLink(linkToTemplate(template.organization_name, templateName)),
					);
				}}
				onSubmit={(templateSettings) => {
					// Exit node bindings are managed through the CLI and API, not
					// this form. Omitting these fields keeps the existing binding.
					updateTemplate({
						...templateMetaWithoutExitNodeBindings(template),
						...templateSettings,
					});
				}}
				accessControlEnabled={accessControlEnabled}
				advancedSchedulingEnabled={advancedSchedulingEnabled}
				sharedPortControlsEnabled={sharedPortControlsEnabled}
			/>
		</>
	);
};

export default TemplateSettingsPage;
