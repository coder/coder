import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type {
	Organization,
	ShareableWorkspaceOwners,
	UpdateOrganizationRequest,
} from "api/typesGenerated";
import { useFormik } from "formik";
import { type FC, useState } from "react";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { FeatureStageBadge } from "#/components/FeatureStageBadge/FeatureStageBadge";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { IconField } from "#/components/IconField/IconField";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
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

	const [isDeleting, setIsDeleting] = useState(false);
	const [pendingSharingChange, setPendingSharingChange] =
		useState<ShareableWorkspaceOwners | null>(null);

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Settings</SettingsHeaderTitle>
			</SettingsHeader>

			{Boolean(error) && !isApiValidationError(error) && (
				<div css={{ marginBottom: 32 }}>
					<ErrorAlert error={error} />
				</div>
			)}

			<HorizontalForm
				onSubmit={form.handleSubmit}
				aria-label="Organization settings form"
			>
				<FormSection
					title="Info"
					description="The name and description of the organization."
				>
					<fieldset
						disabled={form.isSubmitting}
						css={{ border: "unset", padding: 0, margin: 0, width: "100%" }}
					>
						<FormFields>
							<TextField
								{...getFieldHelpers("name")}
								onChange={onChangeTrimmed(form)}
								autoFocus
								fullWidth
								label="Slug"
							/>
							<TextField
								{...getFieldHelpers("display_name")}
								fullWidth
								label="Display name"
							/>
							<TextField
								{...getFieldHelpers("description")}
								multiline
								fullWidth
								label="Description"
								rows={2}
							/>
							<IconField
								{...getFieldHelpers("icon")}
								onChange={onChangeTrimmed(form)}
								fullWidth
								onPickEmoji={(value) => form.setFieldValue("icon", value)}
							/>
						</FormFields>
					</fieldset>
				</FormSection>

				<FormFooter>
					<Button type="submit" disabled={form.isSubmitting}>
						<Spinner loading={form.isSubmitting} />
						Save
					</Button>
				</FormFooter>
			</HorizontalForm>

			{onChangeShareableOwners && (
				<HorizontalContainer className="mt-12">
					<HorizontalSection
						title={
							<div className="flex items-center gap-2">
								Workspace Sharing
								<FeatureStageBadge contentType="beta" size="sm" />
							</div>
						}
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
