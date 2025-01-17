import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import FormHelperText from "@mui/material/FormHelperText";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import {
	type Template,
	type UpdateTemplateMeta,
	WorkspaceAppSharingLevels,
} from "api/typesGenerated";
import { PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import {
	StackLabel,
	StackLabelHelperText,
} from "components/StackLabel/StackLabel";
import { type FormikTouched, useFormik } from "formik";
import type { FC } from "react";
import {
	displayNameValidator,
	getFormHelpers,
	iconValidator,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_DESCRIPTION_MESSAGE = `Please enter a description that is no longer than ${MAX_DESCRIPTION_CHAR_LIMIT} characters.`;

export const validationSchema = Yup.object({
	name: nameValidator("Name"),
	display_name: displayNameValidator("Display name"),
	description: Yup.string().max(
		MAX_DESCRIPTION_CHAR_LIMIT,
		MAX_DESCRIPTION_MESSAGE,
	),
	allow_user_cancel_workspace_jobs: Yup.boolean(),
	icon: iconValidator,
	require_active_version: Yup.boolean(),
	deprecation_message: Yup.string(),
	max_port_sharing_level: Yup.string().oneOf(WorkspaceAppSharingLevels),
});

export interface TemplateSettingsForm {
	template: Template;
	onSubmit: (data: UpdateTemplateMeta) => void;
	onCancel: () => void;
	isSubmitting: boolean;
	error?: unknown;
	// Helpful to show field errors on Storybook
	initialTouched?: FormikTouched<UpdateTemplateMeta>;
	accessControlEnabled: boolean;
	advancedSchedulingEnabled: boolean;
	portSharingControlsEnabled: boolean;
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
	template,
	onSubmit,
	onCancel,
	error,
	isSubmitting,
	initialTouched,
	accessControlEnabled,
	advancedSchedulingEnabled,
	portSharingControlsEnabled,
}) => {
	const form = useFormik<UpdateTemplateMeta>({
		initialValues: {
			name: template.name,
			display_name: template.display_name,
			description: template.description,
			icon: template.icon,
			allow_user_cancel_workspace_jobs:
				template.allow_user_cancel_workspace_jobs,
			update_workspace_last_used_at: false,
			update_workspace_dormant_at: false,
			require_active_version: template.require_active_version,
			deprecation_message: template.deprecation_message,
			disable_everyone_group_access: false,
			max_port_share_level: template.max_port_share_level,
		},
		validationSchema,
		onSubmit,
		initialTouched,
	});
	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<HorizontalForm
			onSubmit={form.handleSubmit}
			aria-label="Template settings form"
		>
			<FormSection
				title="General info"
				description="The name is used to identify the template in URLs and the API."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("name")}
						disabled={isSubmitting}
						onChange={onChangeTrimmed(form)}
						autoFocus
						fullWidth
						label="Name"
					/>
				</FormFields>
			</FormSection>

			<FormSection
				title="Display info"
				description="A friendly name, description, and icon to help developers identify your template."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("display_name")}
						disabled={isSubmitting}
						fullWidth
						label="Display name"
					/>

					<TextField
						{...getFieldHelpers("description", {
							maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
						})}
						multiline
						disabled={isSubmitting}
						fullWidth
						label="Description"
						rows={2}
					/>

					<IconField
						{...getFieldHelpers("icon")}
						disabled={isSubmitting}
						onChange={onChangeTrimmed(form)}
						fullWidth
						label="Icon"
						onPickEmoji={(value) => form.setFieldValue("icon", value)}
					/>
				</FormFields>
			</FormSection>

			<FormSection
				title="Operations"
				description="Regulate actions allowed on workspaces created from this template."
			>
				<FormFields spacing={6}>
					<FormControlLabel
						control={
							<Checkbox
								size="small"
								id="allow_user_cancel_workspace_jobs"
								name="allow_user_cancel_workspace_jobs"
								disabled={isSubmitting}
								checked={form.values.allow_user_cancel_workspace_jobs}
								onChange={form.handleChange}
							/>
						}
						label={
							<StackLabel>
								Allow users to cancel in-progress workspace jobs.
								<StackLabelHelperText>
									Depending on your template, canceling builds may leave
									workspaces in an unhealthy state. This option isn&apos;t
									recommended for most use cases.{" "}
									<strong>
										If checked, users may be able to corrupt their workspace.
									</strong>
								</StackLabelHelperText>
							</StackLabel>
						}
					/>

					<FormControlLabel
						control={
							<Checkbox
								size="small"
								id="require_active_version"
								name="require_active_version"
								checked={form.values.require_active_version}
								onChange={form.handleChange}
								disabled={
									!template.require_active_version && !advancedSchedulingEnabled
								}
							/>
						}
						label={
							<StackLabel>
								Require workspaces automatically update when started.
								<StackLabelHelperText>
									<span>
										Workspaces that are manually started or auto-started will
										use the active template version.{" "}
										<strong>
											This setting is not enforced for template admins.
										</strong>
									</span>

									{!advancedSchedulingEnabled && (
										<Stack
											direction="row"
											spacing={2}
											alignItems="center"
											css={{ marginTop: 16 }}
										>
											<PremiumBadge />
											<span>Premium license required to be enabled.</span>
										</Stack>
									)}
								</StackLabelHelperText>
							</StackLabel>
						}
					/>
				</FormFields>
			</FormSection>

			<FormSection
				title="Deprecate"
				description="Deprecating a template prevents any new workspaces from being created. Existing workspaces will continue to function."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("deprecation_message", {
							helperText:
								"Leave the message empty to keep the template active. Any message provided will mark the template as deprecated. Use this message to inform users of the deprecation and how to migrate to a new template.",
						})}
						disabled={
							isSubmitting || (!template.deprecated && !accessControlEnabled)
						}
						fullWidth
						label="Deprecation Message"
					/>
					{!accessControlEnabled && (
						<Stack direction="row" spacing={2} alignItems="center">
							<PremiumBadge />
							<FormHelperText>
								Premium license required to deprecate templates.
								{template.deprecated &&
									" You cannot change the message, but you may remove it to mark this template as no longer deprecated."}
							</FormHelperText>
						</Stack>
					)}
				</FormFields>
			</FormSection>

			<FormSection
				title="Port Sharing"
				description="Shared ports with the Public sharing level can be accessed by anyone,
          while ports with the Authenticated sharing level can only be accessed
          by authenticated Coder users. Ports with the Owner sharing level can
          only be accessed by the workspace owner."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("max_port_share_level", {
							helperText:
								"The maximum level of port sharing allowed for workspaces.",
						})}
						disabled={isSubmitting || !portSharingControlsEnabled}
						fullWidth
						select
						value={
							portSharingControlsEnabled
								? form.values.max_port_share_level
								: "public"
						}
						label="Maximum Port Sharing Level"
					>
						<MenuItem value="owner">Owner</MenuItem>
						<MenuItem value="authenticated">Authenticated</MenuItem>
						<MenuItem value="public">Public</MenuItem>
					</TextField>
					{!portSharingControlsEnabled && (
						<Stack direction="row" spacing={2} alignItems="center">
							<PremiumBadge />
							<FormHelperText>
								Premium license required to control max port sharing level.
							</FormHelperText>
						</Stack>
					)}
				</FormFields>
			</FormSection>

			<FormFooter>
				<Button onClick={onCancel} variant="outline">
					Cancel
				</Button>

				<Button type="submit" disabled={isSubmitting}>
					<Spinner loading={isSubmitting} />
					Save
				</Button>
			</FormFooter>
		</HorizontalForm>
	);
};
