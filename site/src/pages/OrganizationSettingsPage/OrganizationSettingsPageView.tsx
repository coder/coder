import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type {
	Organization,
	UpdateOrganizationRequest,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import { type FC, useState } from "react";
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
}

export const OrganizationSettingsPageView: FC<
	OrganizationSettingsPageViewProps
> = ({ organization, error, onSubmit, onDeleteOrganization }) => {
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

	return (
		<div>
			<SettingsHeader title="Settings" />
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
