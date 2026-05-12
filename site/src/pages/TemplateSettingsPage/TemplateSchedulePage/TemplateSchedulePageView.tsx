import type { ComponentProps, FC } from "react";
import type { Template, UpdateTemplateMeta } from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { TemplateScheduleForm } from "./TemplateScheduleForm";

interface TemplateSchedulePageViewProps {
	template: Template;
	onSubmit: (data: UpdateTemplateMeta) => void;
	onCancel: () => void;
	isSubmitting: boolean;
	submitError?: unknown;
	initialTouched?: ComponentProps<
		typeof TemplateScheduleForm
	>["initialTouched"];
	allowAdvancedScheduling: boolean;
}

export const TemplateSchedulePageView: FC<TemplateSchedulePageViewProps> = ({
	template,
	onCancel,
	onSubmit,
	isSubmitting,
	allowAdvancedScheduling,
	submitError,
	initialTouched,
}) => {
	return (
		<div className="flex flex-col gap-12">
			<SettingsHeader>
				<SettingsHeaderTitle>Schedule</SettingsHeaderTitle>
			</SettingsHeader>

			<TemplateScheduleForm
				allowAdvancedScheduling={allowAdvancedScheduling}
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
