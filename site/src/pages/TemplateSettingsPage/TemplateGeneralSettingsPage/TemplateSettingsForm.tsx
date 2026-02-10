import {
	CORSBehaviors,
	type Template,
	type UpdateTemplateMeta,
	WorkspaceAppSharingLevels,
} from "api/typesGenerated";
import { PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import { Textarea } from "components/Textarea/Textarea";
import { type FormikTouched, useFormik } from "formik";
import type { FC } from "react";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
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
	use_classic_parameter_flow: Yup.boolean(),
	disable_module_cache: Yup.boolean(),
	deprecation_message: Yup.string(),
	max_port_sharing_level: Yup.string().oneOf(WorkspaceAppSharingLevels),
	cors_behavior: Yup.string().oneOf(Object.values(CORSBehaviors)),
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
			use_classic_parameter_flow: template.use_classic_parameter_flow,
			cors_behavior: template.cors_behavior,
			disable_module_cache: template.disable_module_cache,
		},
		validationSchema,
		onSubmit,
		initialTouched,
	});
	const getFieldHelpers = getFormHelpers(form, error);

	const nameField = getFieldHelpers("name");
	const displayNameField = getFieldHelpers("display_name");
	const descriptionField = getFieldHelpers("description", {
		maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
	});
	const deprecationField = getFieldHelpers("deprecation_message", {
		helperText:
			"Leave the message empty to keep the template active. Any message provided will mark the template as deprecated. Use this message to inform users of the deprecation and how to migrate to a new template.",
	});
	const portShareField = getFieldHelpers("max_port_share_level", {
		helperText: "The maximum level of port sharing allowed for workspaces.",
	});
	const corsField = getFieldHelpers("cors_behavior", {
		helperText: "Use Passthru to bypass Coder's built-in CORS protection.",
	});

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
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={nameField.id}>Name</Label>
						<Input
							id={nameField.id}
							name={nameField.name}
							value={nameField.value}
							onChange={onChangeTrimmed(form)}
							onBlur={nameField.onBlur}
							disabled={isSubmitting}
							autoFocus
							aria-invalid={nameField.error}
						/>
						{nameField.helperText && (
							<span
								className={cn(
									"text-xs",
									nameField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{nameField.helperText}
							</span>
						)}
					</div>
				</FormFields>
			</FormSection>

			<FormSection
				title="Display info"
				description="A friendly name, description, and icon to help developers identify your template."
			>
				<FormFields>
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={displayNameField.id}>Display name</Label>
						<Input
							id={displayNameField.id}
							name={displayNameField.name}
							value={displayNameField.value}
							onChange={displayNameField.onChange}
							onBlur={displayNameField.onBlur}
							disabled={isSubmitting}
							aria-invalid={displayNameField.error}
						/>
						{displayNameField.helperText && (
							<span
								className={cn(
									"text-xs",
									displayNameField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{displayNameField.helperText}
							</span>
						)}
					</div>

					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={descriptionField.id}>Description</Label>
						<Textarea
							id={descriptionField.id}
							name={descriptionField.name}
							value={descriptionField.value}
							onChange={descriptionField.onChange}
							onBlur={descriptionField.onBlur}
							disabled={isSubmitting}
							rows={2}
							aria-invalid={descriptionField.error}
						/>
						{descriptionField.helperText && (
							<span
								className={cn(
									"text-xs",
									descriptionField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{descriptionField.helperText}
							</span>
						)}
					</div>

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
					<div className="flex items-start gap-3">
						<Checkbox
							id="allow_user_cancel_workspace_jobs"
							checked={form.values.allow_user_cancel_workspace_jobs}
							onCheckedChange={(checked) =>
								form.setFieldValue(
									"allow_user_cancel_workspace_jobs",
									checked === true,
								)
							}
							disabled={isSubmitting}
							className="mt-0.5"
						/>
						<label
							htmlFor="allow_user_cancel_workspace_jobs"
							className="flex flex-col gap-1 cursor-pointer"
						>
							<span className="text-sm font-medium">
								Allow users to cancel in-progress workspace jobs.
							</span>
							<span className="text-xs text-content-secondary">
								Depending on your template, canceling builds may leave
								workspaces in an unhealthy state. This option isn&apos;t
								recommended for most use cases.{" "}
								<strong className="text-content-primary">
									If checked, users may be able to corrupt their workspace.
								</strong>
							</span>
						</label>
					</div>

					<div className="flex items-start gap-3">
						<Checkbox
							id="require_active_version"
							checked={form.values.require_active_version}
							onCheckedChange={(checked) =>
								form.setFieldValue("require_active_version", checked === true)
							}
							disabled={
								!template.require_active_version && !advancedSchedulingEnabled
							}
							className="mt-0.5"
						/>
						<label
							htmlFor="require_active_version"
							className="flex flex-col gap-1 cursor-pointer"
						>
							<span className="text-sm font-medium">
								Require workspaces automatically update when started.
							</span>
							<span className="text-xs text-content-secondary">
								Workspaces that are manually started or auto-started will use
								the active template version.{" "}
								<strong className="text-content-primary">
									This setting is not enforced for template admins.
								</strong>
							</span>
							{!advancedSchedulingEnabled && (
								<div className="flex items-center gap-2 mt-2">
									<PremiumBadge />
									<span className="text-xs text-content-secondary">
										Premium license required to be enabled.
									</span>
								</div>
							)}
						</label>
					</div>

					<div className="flex items-start gap-3">
						<Checkbox
							id="use_classic_parameter_flow"
							checked={!form.values.use_classic_parameter_flow}
							onCheckedChange={(checked) =>
								form.setFieldValue(
									"use_classic_parameter_flow",
									checked !== true,
								)
							}
							className="mt-0.5"
						/>
						<label
							htmlFor="use_classic_parameter_flow"
							className="flex flex-col gap-1 cursor-pointer"
						>
							<span className="text-sm font-medium">
								Enable dynamic parameters for workspace creation (recommended)
							</span>
							<span className="text-xs text-content-secondary">
								The dynamic workspace form allows you to design your template
								with additional form types and identity-aware conditional
								parameters. This is the default option for new templates. The
								classic workspace creation flow will be deprecated in a future
								release.{" "}
								<Link
									className="text-xs inline-flex items-start pl-0"
									href={docs(
										"/admin/templates/extending-templates/dynamic-parameters",
									)}
									target="_blank"
								>
									Learn more
								</Link>
							</span>
						</label>
					</div>

					<div className="flex items-start gap-3">
						<Checkbox
							id="disable_module_cache"
							checked={form.values.disable_module_cache}
							onCheckedChange={(checked) =>
								form.setFieldValue("disable_module_cache", checked === true)
							}
							disabled={isSubmitting}
							className="mt-0.5"
						/>
						<label
							htmlFor="disable_module_cache"
							className="flex flex-col gap-1 cursor-pointer"
						>
							<span className="text-sm font-medium">
								Disable Terraform module caching
							</span>
							<span className="text-xs text-content-secondary">
								When checked, Terraform modules are re-downloaded for each
								workspace build instead of using cached versions.{" "}
								<strong className="text-content-primary">
									Warning: This makes workspace builds less predictable and is
									not recommended for production templates.
								</strong>
							</span>
						</label>
					</div>
				</FormFields>
			</FormSection>

			<FormSection
				title="Deprecate"
				description="Deprecating a template prevents any new workspaces from being created. Existing workspaces will continue to function."
			>
				<FormFields>
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={deprecationField.id}>Deprecation Message</Label>
						<Input
							id={deprecationField.id}
							name={deprecationField.name}
							value={deprecationField.value}
							onChange={deprecationField.onChange}
							onBlur={deprecationField.onBlur}
							disabled={
								isSubmitting || (!template.deprecated && !accessControlEnabled)
							}
							aria-invalid={deprecationField.error}
						/>
						{deprecationField.helperText && (
							<span
								className={cn(
									"text-xs",
									deprecationField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{deprecationField.helperText}
							</span>
						)}
					</div>
					{!accessControlEnabled && (
						<div className="flex items-center gap-2">
							<PremiumBadge />
							<span className="text-xs text-content-secondary">
								Premium license required to deprecate templates.
								{template.deprecated &&
									" You cannot change the message, but you may remove it to mark this template as no longer deprecated."}
							</span>
						</div>
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
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={portShareField.id}>
							Maximum Port Sharing Level
						</Label>
						<Select
							value={
								portSharingControlsEnabled
									? form.values.max_port_share_level
									: "public"
							}
							onValueChange={(value) =>
								form.setFieldValue("max_port_share_level", value)
							}
							disabled={isSubmitting || !portSharingControlsEnabled}
						>
							<SelectTrigger id={portShareField.id}>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="owner">Owner</SelectItem>
								<SelectItem value="organization">Organization</SelectItem>
								<SelectItem value="authenticated">Authenticated</SelectItem>
								<SelectItem value="public">Public</SelectItem>
							</SelectContent>
						</Select>
						{portShareField.helperText && (
							<span
								className={cn(
									"text-xs",
									portShareField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{portShareField.helperText}
							</span>
						)}
					</div>
					{!portSharingControlsEnabled && (
						<div className="flex items-center gap-2">
							<PremiumBadge />
							<span className="text-xs text-content-secondary">
								Premium license required to control max port sharing level.
							</span>
						</div>
					)}
				</FormFields>
			</FormSection>

			<FormSection
				title="CORS Behavior"
				description="Control how Cross-Origin Resource Sharing (CORS) requests are handled for all shared ports."
			>
				<FormFields>
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={corsField.id}>CORS Behavior</Label>
						<Select
							value={form.values.cors_behavior}
							onValueChange={(value) =>
								form.setFieldValue("cors_behavior", value)
							}
							disabled={isSubmitting}
						>
							<SelectTrigger id={corsField.id}>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="simple">Simple (recommended)</SelectItem>
								<SelectItem value="passthru">Passthru</SelectItem>
							</SelectContent>
						</Select>
						{corsField.helperText && (
							<span
								className={cn(
									"text-xs",
									corsField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{corsField.helperText}
							</span>
						)}
					</div>
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
