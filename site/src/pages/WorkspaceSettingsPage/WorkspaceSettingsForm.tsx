import { useFormik } from "formik";
import upperFirst from "lodash/upperFirst";
import type { FC } from "react";
import * as Yup from "yup";
import {
	type AutomaticUpdates,
	AutomaticUpdateses,
	type Workspace,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

export type WorkspaceSettingsFormValues = {
	name: string;
	automatic_updates: AutomaticUpdates;
};

interface WorkspaceSettingsFormProps {
	workspace: Workspace;
	error: unknown;
	onCancel: () => void;
	onSubmit: (values: WorkspaceSettingsFormValues) => Promise<void>;
}

export const WorkspaceSettingsForm: FC<WorkspaceSettingsFormProps> = ({
	onCancel,
	onSubmit,
	workspace,
	error,
}) => {
	const formEnabled =
		!workspace.template_require_active_version || workspace.allow_renames;

	const form = useFormik<WorkspaceSettingsFormValues>({
		onSubmit,
		initialValues: {
			name: workspace.name,
			automatic_updates: workspace.automatic_updates,
		},
		validationSchema: Yup.object({
			name: nameValidator("Name"),
			automatic_updates: Yup.string().oneOf(AutomaticUpdateses),
		}),
	});
	const getFieldHelpers = getFormHelpers<WorkspaceSettingsFormValues>(
		form,
		error,
	);

	const nameField = getFieldHelpers("name");
	const nameDisabled = !workspace.allow_renames || form.isSubmitting;
	const nameHelperText = workspace.allow_renames
		? form.values.name !== form.initialValues.name &&
			"Depending on the template, renaming your workspace may be destructive"
		: "Renaming your workspace can be destructive and is disabled by the template.";

	const automaticUpdatesValue = workspace.template_require_active_version
		? "always"
		: form.values.automatic_updates;
	const automaticUpdatesDisabled =
		form.isSubmitting || workspace.template_require_active_version;

	return (
		<HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
			<FormSection
				title="Workspace Name"
				description="Update the name of your workspace."
			>
				<FormFields>
					<div className="flex flex-col gap-2">
						<Label htmlFor="name">Name</Label>
						<Input
							id="name"
							name={nameField.name}
							autoFocus
							disabled={nameDisabled}
							value={nameField.value ?? ""}
							onChange={onChangeTrimmed(form)}
							onBlur={nameField.onBlur}
							aria-invalid={nameField.error}
						/>
						{nameHelperText && (
							<span
								className={cn(
									"text-xs",
									workspace.allow_renames
										? "text-content-warning"
										: "text-content-secondary",
								)}
							>
								{nameHelperText}
							</span>
						)}
					</div>
				</FormFields>
			</FormSection>
			<FormSection
				title="Automatic Updates"
				description="Configure your workspace to automatically update when started."
			>
				<FormFields>
					<div className="flex flex-col gap-2">
						<Label htmlFor="automatic_updates">Update Policy</Label>
						<Select
							value={automaticUpdatesValue}
							onValueChange={(value) => {
								void form.setFieldValue("automatic_updates", value);
							}}
							disabled={automaticUpdatesDisabled}
						>
							<SelectTrigger id="automatic_updates">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{AutomaticUpdateses.map((value) => (
									<SelectItem value={value} key={value}>
										{upperFirst(value)}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						{workspace.template_require_active_version && (
							<span className="text-xs text-content-secondary">
								The template for this workspace requires automatic updates.
							</span>
						)}
					</div>
				</FormFields>
			</FormSection>
			{formEnabled && (
				<FormFooter>
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>

					<Button type="submit" disabled={form.isSubmitting}>
						<Spinner loading={form.isSubmitting} />
						Save
					</Button>
				</FormFooter>
			)}
		</HorizontalForm>
	);
};
