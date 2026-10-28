import type { Template, UpdateTemplateMeta } from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { TemplateScheduleForm } from "./TemplateScheduleForm";

type TemplateSchedulePageViewProps = {
	template: Template;
	onSubmit: (data: UpdateTemplateMeta) => void;
	onCancel: () => void;
	isSubmitting: boolean;
	submitError?: unknown;
	initialTouched?: React.ComponentProps<
		typeof TemplateScheduleForm
	>["initialTouched"];
	allowAdvancedScheduling: boolean;
	/** Shows Premium page links in Premium notices. */
	canViewPremium: boolean;
};

export const TemplateSchedulePageView: React.FC<
	TemplateSchedulePageViewProps
> = ({
	template,
	onCancel,
	onSubmit,
	isSubmitting,
	allowAdvancedScheduling,
	canViewPremium,
	submitError,
	initialTouched,
}) => {
	return (
		<div className="flex flex-col gap-12">
			<SettingsHeader>
				<SettingsHeaderTitle>Schedule</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure workspace schedule defaults for this template.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<TemplateScheduleForm
				allowAdvancedScheduling={allowAdvancedScheduling}
				canViewPremium={canViewPremium}
				initialTouched={initialTouched}
				isSubmitting={isSubmitting}
				template={template}
				onSubmit={onSubmit}
				onCancel={onCancel}
				error={submitError}
			/>
		</div>
	);
};
