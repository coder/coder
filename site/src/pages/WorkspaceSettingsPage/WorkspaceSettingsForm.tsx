import type { Theme } from "@emotion/react";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import {
	type AutomaticUpdates,
	AutomaticUpdateses,
	type Workspace,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import upperFirst from "lodash/upperFirst";
import type { FC } from "react";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

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

	return (
		<HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
			<FormSection
				title="Workspace Name"
				description="Update the name of your workspace."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("name")}
						disabled={!workspace.allow_renames || form.isSubmitting}
						onChange={onChangeTrimmed(form)}
						autoFocus
						fullWidth
						label="Name"
						css={workspace.allow_renames && styles.nameWarning}
						helperText={
							workspace.allow_renames
								? form.values.name !== form.initialValues.name &&
									"Depending on the template, renaming your workspace may be destructive"
								: "Renaming your workspace can be destructive and is disabled by the template."
						}
					/>
				</FormFields>
			</FormSection>
			<FormSection
				title="Automatic Updates"
				description="Configure your workspace to automatically update when started."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("automatic_updates")}
						id="automatic_updates"
						label="Update Policy"
						value={
							workspace.template_require_active_version
								? "always"
								: form.values.automatic_updates
						}
						select
						disabled={
							form.isSubmitting || workspace.template_require_active_version
						}
						helperText={
							workspace.template_require_active_version &&
							"The template for this workspace requires automatic updates."
						}
					>
						{AutomaticUpdateses.map((value) => (
							<MenuItem value={value} key={value}>
								{upperFirst(value)}
							</MenuItem>
						))}
					</TextField>
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

const styles = {
	nameWarning: (theme: Theme) => ({
		"& .MuiFormHelperText-root": {
			color: theme.palette.warning.light,
		},
	}),
};
