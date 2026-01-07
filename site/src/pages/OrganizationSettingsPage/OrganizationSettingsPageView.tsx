import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type {
	Organization,
	UpdateOrganizationRequest,
	WorkspaceSharingSettings,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { Label } from "components/Label/Label";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import { useFormik } from "formik";
import { type FC, useId, useState } from "react";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
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
	workspaceSharingSettings?: WorkspaceSharingSettings;
	onUpdateWorkspaceSharingSettings?: (
		settings: WorkspaceSharingSettings,
	) => Promise<void>;
	isUpdatingWorkspaceSharingSettings?: boolean;
	canEditWorkspaceSharingSettings?: boolean;
	showWorkspaceSharingSettings?: boolean;
}

export const OrganizationSettingsPageView: FC<
	OrganizationSettingsPageViewProps
> = ({
	organization,
	error,
	onSubmit,
	onDeleteOrganization,
	workspaceSharingSettings,
	onUpdateWorkspaceSharingSettings,
	isUpdatingWorkspaceSharingSettings,
	canEditWorkspaceSharingSettings,
	showWorkspaceSharingSettings,
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
	const [showDisableSharingDialog, setShowDisableSharingDialog] =
		useState(false);
	const workspaceSharingId = useId();

	const handleWorkspaceSharingToggle = async (checked: boolean) => {
		if (!onUpdateWorkspaceSharingSettings) return;

		// If disabling sharing (turning toggle OFF), show confirmation dialog.
		if (!checked) {
			setShowDisableSharingDialog(true);
			return;
		}

		// If enabling sharing (turning toggle ON), update immediately.
		await onUpdateWorkspaceSharingSettings({ sharing_disabled: false });
	};

	const confirmDisableSharing = async () => {
		if (!onUpdateWorkspaceSharingSettings) return;
		await onUpdateWorkspaceSharingSettings({ sharing_disabled: true });
		setShowDisableSharingDialog(false);
	};

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

			{showWorkspaceSharingSettings && (
				<HorizontalContainer className="mt-12">
					<HorizontalSection
						title="Workspace sharing"
						description="Control whether users in this organization can share workspaces with other users and groups."
					>
						<div className="flex items-center justify-between border border-solid border-border rounded-md p-3 pl-4 gap-4 flex-grow">
							<div className="flex flex-col gap-1">
								<Label
									htmlFor={`${workspaceSharingId}-enable-sharing`}
									className="cursor-pointer"
								>
									Enable workspace sharing
								</Label>
								<p className="text-sm text-content-secondary m-0">
									When enabled, users can share workspaces with other users and
									groups.
								</p>
							</div>
							<Spinner
								size="sm"
								loading={isUpdatingWorkspaceSharingSettings}
								className="w-9"
							>
								<Switch
									id={`${workspaceSharingId}-enable-sharing`}
									checked={!(workspaceSharingSettings?.sharing_disabled ?? true)}
									onCheckedChange={handleWorkspaceSharingToggle}
									disabled={
										!canEditWorkspaceSharingSettings ||
										isUpdatingWorkspaceSharingSettings
									}
								/>
							</Spinner>
						</div>
					</HorizontalSection>
				</HorizontalContainer>
			)}

			<ConfirmDialog
				type="delete"
				hideCancel={false}
				open={showDisableSharingDialog}
				title="Disable workspace sharing"
				onConfirm={confirmDisableSharing}
				onClose={() => setShowDisableSharingDialog(false)}
				confirmLoading={isUpdatingWorkspaceSharingSettings}
				confirmText="Disable sharing"
				description={
					<>
						<p>
							<strong>Warning:</strong> Disabling workspace sharing is a
							destructive action. All existing workspace sharing permissions
							will be permanently removed for all workspaces in this
							organization.
						</p>
						<p>
							Users will no longer be able to share workspaces, and any existing
							shared access will be revoked immediately.
						</p>
						<p>Are you sure you want to continue?</p>
					</>
				}
			/>

			{!organization.is_default && (
				<HorizontalContainer className="mt-12">
					<HorizontalSection
						title="Settings"
						description="Change or delete your organization."
					>
						<div className="flex bg-surface-orange items-center justify-between border border-solid border-orange-600 rounded-md p-3 pl-4 gap-2 flex-grow">
							<span>Deleting an organization is irreversible.</span>
							<Button
								variant="destructive"
								onClick={() => setIsDeleting(true)}
								className="min-w-fit"
							>
								Delete this organization
							</Button>
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
		</div>
	);
};
