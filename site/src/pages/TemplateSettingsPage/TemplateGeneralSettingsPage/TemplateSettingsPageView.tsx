import type { Template, UpdateTemplateMeta } from "api/typesGenerated";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import type { ComponentProps, FC } from "react";
import { TemplateSettingsForm } from "./TemplateSettingsForm";

interface TemplateSettingsPageViewProps {
	template: Template;
	onSubmit: (data: UpdateTemplateMeta) => void;
	onCancel: () => void;
	isSubmitting: boolean;
	submitError?: unknown;
	initialTouched?: ComponentProps<
		typeof TemplateSettingsForm
	>["initialTouched"];
	accessControlEnabled: boolean;
	advancedSchedulingEnabled: boolean;
	sharedPortControlsEnabled: boolean;
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageViewProps> = ({
	template,
	onCancel,
	onSubmit,
	isSubmitting,
	submitError,
	initialTouched,
	accessControlEnabled,
	advancedSchedulingEnabled,
	sharedPortControlsEnabled,
}) => {
	return (
		<>
			<PageHeader className="pt-0">
				<PageHeaderTitle>General Settings</PageHeaderTitle>
			</PageHeader>

			<TemplateSettingsForm
				initialTouched={initialTouched}
				isSubmitting={isSubmitting}
				template={template}
				onSubmit={onSubmit}
				onCancel={onCancel}
				error={submitError}
				accessControlEnabled={accessControlEnabled}
				advancedSchedulingEnabled={advancedSchedulingEnabled}
				portSharingControlsEnabled={sharedPortControlsEnabled}
			/>
		</>
	);
};
