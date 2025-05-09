import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import { templateByNameKey } from "api/queries/templates";
import type { UpdateTemplateMeta } from "api/typesGenerated";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useDashboard } from "modules/dashboard/useDashboard";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateSettingsPageView } from "./TemplateSettingsPageView";

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
		isLoading: isSubmitting,
		error: submitError,
	} = useMutation(
		(data: UpdateTemplateMeta) => {
			return API.updateTemplateMeta(template.id, data);
		},
		{
			onSuccess: async (data) => {
				// This update has a chance to return a 304 which means nothing was updated.
				// In this case, the return payload will be empty and we should use the
				// original template data.
				if (!data) {
					data = template;
				} else {
					// Only invalid the query if data is returned, indicating at least one field was updated.
					//
					// we use data.name because an admin may have updated templateName to something new
					await queryClient.invalidateQueries(
						templateByNameKey(template.organization_name, data.name),
					);
				}
				displaySuccess("Template updated successfully");
				navigate(getLink(linkToTemplate(data.organization_name, data.name)));
			},
			onError: (error) => {
				displayError(getErrorMessage(error, "Failed to update template"));
			},
		},
	);

	return (
		<>
			<Helmet>
				<title>{pageTitle(template.name, "General Settings")}</title>
			</Helmet>
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
					updateTemplate({
						...template,
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
