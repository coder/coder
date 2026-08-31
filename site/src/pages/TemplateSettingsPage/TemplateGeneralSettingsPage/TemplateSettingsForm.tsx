import { type FormikTouched, useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import {
	CORSBehaviors,
	type Template,
	type UpdateTemplateMeta,
	WorkspaceAppSharingLevels,
} from "#/api/typesGenerated";
import { PremiumBadge } from "#/components/Badges/Badges";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StackLabel,
	StackLabelHelperText,
} from "#/components/StackLabel/StackLabel";
import { Textarea } from "#/components/Textarea/Textarea";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";
import {
	displayNameValidator,
	getFormHelpers,
	iconValidator,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

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
	agents_allowed: Yup.boolean(),
	allow_workspace_renames: Yup.boolean(),
	icon: iconValidator,
	require_active_version: Yup.boolean(),
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
			agents_allowed: template.agents_allowed,
			update_workspace_last_used_at: false,
			update_workspace_dormant_at: false,
			require_active_version: template.require_active_version,
			deprecation_message: template.deprecation_message,
			disable_everyone_group_access: false,
			max_port_share_level: template.max_port_share_level,
			cors_behavior: template.cors_behavior,
			disable_module_cache: template.disable_module_cache,
			allow_workspace_renames: template.allow_workspace_renames,
		},
		validationSchema,
		onSubmit,
		initialTouched,
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const descriptionField = getFieldHelpers("description", {
		maxLength: MAX_DESCRIPTION_CHAR_LIMIT,
	});
	const descriptionHelperId = `${descriptionField.id}-helper`;
	const maxPortShareField = getFieldHelpers("max_port_share_level", {
		helperText: "The maximum level of port sharing allowed for workspaces.",
	});
	const maxPortShareHelperId = `${maxPortShareField.id}-helper`;
	const corsBehaviorField = getFieldHelpers("cors_behavior", {
		helperText: "Use Passthru to bypass Coder's built-in CORS protection.",
	});
	const corsBehaviorHelperId = `${corsBehaviorField.id}-helper`;

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
					<FormField
						field={getFieldHelpers("name")}
						label="Name"
						disabled={isSubmitting}
						onChange={onChangeTrimmed(form)}
						autoFocus
						className="w-full"
					/>
				</FormFields>
			</FormSection>

			<FormSection
				title="Display info"
				description="A friendly name, description, and icon to help developers identify your template."
			>
				<FormFields>
					<FormField
						field={getFieldHelpers("display_name")}
						label="Display name"
						disabled={isSubmitting}
						className="w-full"
					/>

					<div className="flex flex-col gap-2">
						<Label htmlFor={descriptionField.id}>Description</Label>
						<Textarea
							id={descriptionField.id}
							name={descriptionField.name}
							value={descriptionField.value ?? ""}
							onChange={descriptionField.onChange}
							onBlur={descriptionField.onBlur}
							disabled={isSubmitting}
							rows={2}
							aria-invalid={descriptionField.error}
							aria-describedby={
								descriptionField.helperText ? descriptionHelperId : undefined
							}
							className={cn(
								descriptionField.error && "border-border-destructive",
							)}
						/>
						{descriptionField.helperText && (
							<span
								id={descriptionHelperId}
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
				<FormFields className="gap-12">
					<div className="flex items-start">
						<Checkbox
							id="agents_allowed"
							name="agents_allowed"
							disabled={isSubmitting}
							checked={form.values.agents_allowed}
							onCheckedChange={(checked) => {
								form.setFieldValue("agents_allowed", checked === true);
							}}
						/>
						<Label htmlFor="agents_allowed">
							<StackLabel>
								Allow Coder Agents to create workspaces using this template
							</StackLabel>
						</Label>
					</div>

					<div className="flex items-start">
						<Checkbox
							id="allow_user_cancel_workspace_jobs"
							name="allow_user_cancel_workspace_jobs"
							disabled={isSubmitting}
							checked={form.values.allow_user_cancel_workspace_jobs}
							onCheckedChange={(checked) => {
								form.setFieldValue(
									"allow_user_cancel_workspace_jobs",
									checked === true,
								);
							}}
						/>
						<Label htmlFor="allow_user_cancel_workspace_jobs">
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
						</Label>
					</div>

					<div className="flex items-start">
						<Checkbox
							id="require_active_version"
							name="require_active_version"
							checked={form.values.require_active_version}
							onCheckedChange={(checked) => {
								form.setFieldValue("require_active_version", checked === true);
							}}
							disabled={
								!template.require_active_version && !advancedSchedulingEnabled
							}
						/>
						<Label htmlFor="require_active_version">
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
										<div className="flex flex-row gap-4 items-center mt-4">
											<PremiumBadge />
											<span>Premium license required to be enabled.</span>
										</div>
									)}
								</StackLabelHelperText>
							</StackLabel>
						</Label>
					</div>

					<div className="flex items-start">
						<Checkbox
							id="disable_module_cache"
							name="disable_module_cache"
							checked={form.values.disable_module_cache}
							onCheckedChange={(checked) => {
								form.setFieldValue("disable_module_cache", checked === true);
							}}
							disabled={isSubmitting}
						/>
						<Label htmlFor="disable_module_cache">
							<StackLabel>
								Disable Terraform module caching
								<StackLabelHelperText>
									When checked, Terraform modules are re-downloaded for each
									workspace build instead of using cached versions.{" "}
									<strong>
										Warning: This makes workspace builds less predictable and is
										not recommended for production templates.
									</strong>
								</StackLabelHelperText>
							</StackLabel>
						</Label>
					</div>

					<div className="flex items-start">
						<Checkbox
							id="allow_workspace_renames"
							name="allow_workspace_renames"
							checked={form.values.allow_workspace_renames}
							onCheckedChange={(checked) => {
								form.setFieldValue("allow_workspace_renames", checked === true);
							}}
							disabled={isSubmitting}
						/>
						<Label htmlFor="allow_workspace_renames">
							<StackLabel>
								Allow users to rename their workspaces.
								<StackLabelHelperText>
									<div>
										Only enable this if your template does not use the workspace
										name in such a way that changing it may destroy/recreate a
										resource.
									</div>
									<Link
										className="text-xs"
										href={docs(
											"/admin/templates/extending-templates/resource-persistence",
										)}
									>
										Learn more
									</Link>
								</StackLabelHelperText>
							</StackLabel>
						</Label>
					</div>
				</FormFields>
			</FormSection>

			<FormSection
				title="Deprecate"
				description="Deprecating a template prevents any new workspaces from being created. Existing workspaces will continue to function."
			>
				<FormFields>
					<FormField
						field={getFieldHelpers("deprecation_message", {
							helperText:
								"Leave the message empty to keep the template active. Any message provided will mark the template as deprecated. Use this message to inform users of the deprecation and how to migrate to a new template.",
						})}
						label="Deprecation Message"
						disabled={
							isSubmitting || (!template.deprecated && !accessControlEnabled)
						}
						className="w-full"
					/>
					{!accessControlEnabled && (
						<div className="flex flex-row gap-4 items-center">
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
					<div className="flex flex-col gap-2">
						<Label htmlFor={maxPortShareField.id}>
							Maximum Port Sharing Level
						</Label>
						<Select
							value={
								portSharingControlsEnabled
									? form.values.max_port_share_level
									: "public"
							}
							onValueChange={(value) => {
								form.setFieldValue("max_port_share_level", value);
							}}
							disabled={isSubmitting || !portSharingControlsEnabled}
						>
							<SelectTrigger
								id={maxPortShareField.id}
								className={cn(
									"w-full",
									maxPortShareField.error && "border-border-destructive",
								)}
								aria-invalid={maxPortShareField.error}
								aria-describedby={
									maxPortShareField.helperText
										? maxPortShareHelperId
										: undefined
								}
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="owner">Owner</SelectItem>
								<SelectItem value="organization">Organization</SelectItem>
								<SelectItem value="authenticated">Authenticated</SelectItem>
								<SelectItem value="public">Public</SelectItem>
							</SelectContent>
						</Select>
						{maxPortShareField.helperText && (
							<span
								id={maxPortShareHelperId}
								className={cn(
									"text-xs",
									maxPortShareField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{maxPortShareField.helperText}
							</span>
						)}
					</div>
					{!portSharingControlsEnabled && (
						<div className="flex flex-row gap-4 items-center">
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
					<div className="flex flex-col gap-2">
						<Label htmlFor={corsBehaviorField.id}>CORS Behavior</Label>
						<Select
							value={form.values.cors_behavior}
							onValueChange={(value) => {
								form.setFieldValue("cors_behavior", value);
							}}
							disabled={isSubmitting}
						>
							<SelectTrigger
								id={corsBehaviorField.id}
								className={cn(
									"w-full",
									corsBehaviorField.error && "border-border-destructive",
								)}
								aria-invalid={corsBehaviorField.error}
								aria-describedby={
									corsBehaviorField.helperText
										? corsBehaviorHelperId
										: undefined
								}
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="simple">Simple (recommended)</SelectItem>
								<SelectItem value="passthru">Passthru</SelectItem>
							</SelectContent>
						</Select>
						{corsBehaviorField.helperText && (
							<span
								id={corsBehaviorHelperId}
								className={cn(
									"text-xs",
									corsBehaviorField.error
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{corsBehaviorField.helperText}
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
