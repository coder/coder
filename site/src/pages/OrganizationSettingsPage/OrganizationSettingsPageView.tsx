import { useFormik } from "formik";
import { type FC, useState } from "react";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import type {
	Organization,
	ShareableWorkspaceOwners,
	UpdateOrganizationRequest,
} from "#/api/typesGenerated";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { cn } from "#/utils/cn";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";
import { DisableWorkspaceSharingDialog } from "./DisableWorkspaceSharingDialog";
import { HorizontalContainer, HorizontalSection } from "./Horizontal";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_DESCRIPTION_MESSAGE = `Please enter a description that is no longer than ${MAX_DESCRIPTION_CHAR_LIMIT} characters.`;

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	display_name: displayNameValidator("Display name"),
	description: Yup.string().max(
		MAX_DESCRIPTION_CHAR_LIMIT,
		MAX_DESCRIPTION_MESSAGE,
	),
});

interface OrganizationSettingsPageViewProps {
	organization: Organization;
	error: unknown;
	onSubmit: (values: UpdateOrganizationRequest) => Promise<void>;
	onDeleteOrganization: () => void;
	workspaceSharingGloballyDisabled?: boolean;
	shareableWorkspaceOwners: ShareableWorkspaceOwners;
	onChangeShareableOwners: (value: ShareableWorkspaceOwners) => void;
	isTogglingWorkspaceSharing: boolean;
}

export const OrganizationSettingsPageView: FC<
	OrganizationSettingsPageViewProps
> = ({
	organization,
	error,
	onSubmit,
	onDeleteOrganization,
	workspaceSharingGloballyDisabled,
	shareableWorkspaceOwners,
	onChangeShareableOwners,
	isTogglingWorkspaceSharing,
}) => {
	const form = useFormik<UpdateOrganizationRequest>({
		initialValues: {
			name: organization.name,
			display_name: organization.display_name,
			description: organization.description,
			icon: organization.icon,
		},
		validationSchema,
		onSubmit,
		enableReinitialize: true,
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const descriptionField = getFieldHelpers("description");

	const [isDeleting, setIsDeleting] = useState(false);
	const [pendingSharingChange, setPendingSharingChange] =
		useState<ShareableWorkspaceOwners | null>(null);

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Settings</SettingsHeaderTitle>
			</SettingsHeader>

			{Boolean(error) && !isApiValidationError(error) && (
				<div className="mb-8">
					<ErrorAlert error={error} />
				</div>
			)}

			<form
				onSubmit={form.handleSubmit}
				aria-label="Organization settings form"
				className="border border-border border-solid rounded-md p-4"
			>
				<fieldset
					disabled={form.isSubmitting}
					className="border-0 p-0 m-0 w-full"
				>
					<div className="flex flex-col gap-6">
						<FormField
							field={getFieldHelpers("name")}
							label="Slug"
							id="name"
							name="name"
							value={form.values.name}
							onChange={onChangeTrimmed(form)}
							onBlur={form.handleBlur}
							autoFocus
							className="max-w-sm"
						/>

						<FormField
							field={getFieldHelpers("display_name")}
							label="Display name"
							id="display_name"
							name="display_name"
							value={form.values.display_name}
							onChange={form.handleChange}
							onBlur={form.handleBlur}
							className="max-w-sm"
						/>

						<div className="flex flex-col gap-2">
							<Label htmlFor="description">Description</Label>
							<Textarea
								id="description"
								name="description"
								value={form.values.description}
								onChange={form.handleChange}
								onBlur={form.handleBlur}
								rows={2}
								aria-invalid={descriptionField.error}
								aria-describedby={
									descriptionField.error
										? "description-error"
										: descriptionField.helperText
											? "description-helper"
											: undefined
								}
								className={cn(
									descriptionField.error && "border-border-destructive",
									"max-w-sm",
								)}
							/>
							{descriptionField.error ? (
								<span
									id="description-error"
									className="text-xs text-content-destructive"
								>
									{descriptionField.helperText}
								</span>
							) : (
								descriptionField.helperText && (
									<span
										id="description-helper"
										className="text-xs text-content-secondary"
									>
										{descriptionField.helperText}
									</span>
								)
							)}
						</div>

						<div className="max-w-sm">
							<IconField
								{...getFieldHelpers("icon")}
								onChange={onChangeTrimmed(form)}
								onPickEmoji={(value) => form.setFieldValue("icon", value)}
							/>
						</div>
					</div>
				</fieldset>

				<FormFooter className="mt-8">
					<Button type="submit" disabled={form.isSubmitting}>
						<Spinner loading={form.isSubmitting} />
						Save
					</Button>
				</FormFooter>
			</form>

			{onChangeShareableOwners && (
				<HorizontalContainer className="mt-12">
					<HorizontalSection
						title="Workspace Sharing"
						description="Control whether workspace owners can share their workspaces."
					>
						<div className="flex flex-col gap-2">
							{workspaceSharingGloballyDisabled && (
								<Alert severity="warning" className="mb-4">
									<AlertTitle>Disabled by deployment settings</AlertTitle>
									Workspace sharing has been disallowed by an administrator.
									Sharing must be allowed by an administrator before sharing can
									be used in this organization.
								</Alert>
							)}
							<div className="flex items-start gap-3">
								<Checkbox
									id="workspace-sharing"
									checked={
										!workspaceSharingGloballyDisabled &&
										shareableWorkspaceOwners !== "none"
									}
									disabled={
										workspaceSharingGloballyDisabled ||
										isTogglingWorkspaceSharing
									}
									onCheckedChange={(checked) => {
										if (checked) {
											// Default to service_accounts when enabling.
											onChangeShareableOwners("service_accounts");
										} else {
											setPendingSharingChange("none");
										}
									}}
								/>
								<div className="flex flex-col gap-3">
									<div className="flex flex-col">
										<label
											htmlFor="workspace-sharing"
											className="text-sm cursor-pointer"
										>
											Allow workspace sharing
										</label>
										<div className="text-xs text-content-secondary">
											When enabled, workspace owners can share their workspaces
											with other users in this organization.
										</div>
									</div>
									{shareableWorkspaceOwners !== "none" &&
										!workspaceSharingGloballyDisabled && (
											<RadioGroup
												value={shareableWorkspaceOwners}
												onValueChange={(value) => {
													const newValue = value as ShareableWorkspaceOwners;
													// Transitioning from "everyone" to "service_accounts"
													// is destructive, so show the warning dialog.
													// Otherwise, just change.
													if (
														shareableWorkspaceOwners === "everyone" &&
														newValue === "service_accounts"
													) {
														setPendingSharingChange("service_accounts");
													} else {
														onChangeShareableOwners(newValue);
													}
												}}
												disabled={isTogglingWorkspaceSharing}
												className="ml-1"
											>
												<div className="flex items-start gap-2">
													<RadioGroupItem
														value="service_accounts"
														id="sharing-service-accounts"
														className="mt-0.5"
													/>
													<div className="flex flex-col">
														<label
															htmlFor="sharing-service-accounts"
															className="text-sm cursor-pointer"
														>
															Only service accounts can share workspaces
														</label>
														<span className="text-xs text-content-secondary">
															Service accounts are non-login accounts typically
															used for automation, CI/CD pipelines, and
															centrally-managed shared environments.
														</span>
													</div>
												</div>
												<div className="flex items-center gap-2">
													<RadioGroupItem
														value="everyone"
														id="sharing-everyone"
													/>
													<label
														htmlFor="sharing-everyone"
														className="text-sm cursor-pointer"
													>
														All members can share workspaces
													</label>
												</div>
											</RadioGroup>
										)}
								</div>
							</div>
						</div>
					</HorizontalSection>
				</HorizontalContainer>
			)}

			{!organization.is_default && (
				<HorizontalContainer className="mt-12">
					<HorizontalSection
						title="Delete Organization"
						description="Delete your organization permanently."
					>
						<div className="flex flex-col gap-4 flex-grow">
							<div className="flex bg-surface-orange items-center justify-between border border-solid border-orange-600 rounded-md p-3 pl-4 gap-2">
								<span>Deleting an organization is irreversible.</span>
								<Button
									variant="destructive"
									onClick={() => setIsDeleting(true)}
									className="min-w-fit"
								>
									Delete this organization
								</Button>
							</div>
						</div>
					</HorizontalSection>
				</HorizontalContainer>
			)}

			<DeleteDialog
				isOpen={isDeleting}
				onConfirm={async () => {
					await onDeleteOrganization();
					setIsDeleting(false);
				}}
				onCancel={() => setIsDeleting(false)}
				entity="organization"
				name={organization.name}
			/>

			<DisableWorkspaceSharingDialog
				isOpen={pendingSharingChange !== null}
				organizationId={organization.id}
				newSetting={pendingSharingChange ?? "none"}
				onConfirm={async () => {
					if (pendingSharingChange !== null) {
						await onChangeShareableOwners(pendingSharingChange);
					}
					setPendingSharingChange(null);
				}}
				onCancel={() => setPendingSharingChange(null)}
				isLoading={isTogglingWorkspaceSharing}
			/>
		</div>
	);
};
