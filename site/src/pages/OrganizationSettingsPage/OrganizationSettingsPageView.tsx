import type { Interpolation, Theme } from "@emotion/react";
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
				data-testid="org-settings-form"
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
				<HorizontalContainer css={{ marginTop: 48 }}>
					<HorizontalSection
						title="Settings"
						description="Change or delete your organization."
					>
						<div css={styles.dangerSettings}>
							<span>Deleting an organization is irreversible.</span>
							<Button variant="destructive" onClick={() => setIsDeleting(true)}>
								Delete this organization
							</Button>
						</div>
					</HorizontalSection>
				</HorizontalContainer>
			)}

			<DeleteDialog
				isOpen={isDeleting}
				onConfirm={onDeleteOrganization}
				onCancel={() => setIsDeleting(false)}
				entity="organization"
				name={organization.name}
			/>
		</div>
	);
};

const styles = {
	dangerSettings: (theme) => ({
		display: "flex",
		backgroundColor: theme.roles.danger.background,
		alignItems: "center",
		justifyContent: "space-between",
		border: `1px solid ${theme.roles.danger.outline}`,
		borderRadius: 8,
		padding: 12,
		paddingLeft: 18,
		gap: 8,
		lineHeight: "18px",
		flexGrow: 1,

		"& .option": {
			color: theme.roles.danger.fill.solid,
			"&.Mui-checked": {
				color: theme.roles.danger.fill.solid,
			},
		},

		"& .info": {
			fontSize: 14,
			fontWeight: 600,
			color: theme.roles.danger.text,
		},
	}),
	dangerButton: (theme) => ({
		borderColor: theme.roles.danger.outline,
		color: theme.roles.danger.text,

		"&.MuiLoadingButton-loading": {
			color: theme.roles.danger.disabled.text,
		},

		"&:hover:not(:disabled)": {
			backgroundColor: theme.roles.danger.hover.background,
			borderColor: theme.roles.danger.hover.fill.outline,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
