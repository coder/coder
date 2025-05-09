import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type { CreateGroupRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import type { FC } from "react";
import { useNavigate } from "react-router-dom";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
});

export type CreateGroupPageViewProps = {
	onSubmit: (data: CreateGroupRequest) => void;
	error?: unknown;
	isLoading: boolean;
};

export const CreateGroupPageView: FC<CreateGroupPageViewProps> = ({
	onSubmit,
	error,
	isLoading,
}) => {
	const navigate = useNavigate();
	const form = useFormik<CreateGroupRequest>({
		initialValues: {
			name: "",
			display_name: "",
			avatar_url: "",
			quota_allowance: 0,
		},
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, error);
	const onCancel = () => navigate(-1);

	return (
		<>
			<SettingsHeader>
				<SettingsHeaderTitle>New Group</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Create a group in this organization.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<HorizontalForm onSubmit={form.handleSubmit}>
				<FormSection
					title="Group settings"
					description="Set a name and avatar for this group."
				>
					<FormFields>
						{Boolean(error) && !isApiValidationError(error) && (
							<ErrorAlert error={error} />
						)}

						<TextField
							{...getFieldHelpers("name")}
							autoFocus
							fullWidth
							label="Name"
							onChange={onChangeTrimmed(form)}
							autoComplete="name"
						/>
						<TextField
							{...getFieldHelpers("display_name", {
								helperText: "Optional: keep empty to default to the name.",
							})}
							fullWidth
							label="Display Name"
							autoComplete="display_name"
						/>
						<IconField
							{...getFieldHelpers("avatar_url")}
							onChange={onChangeTrimmed(form)}
							fullWidth
							label="Avatar URL"
							onPickEmoji={(value) => form.setFieldValue("avatar_url", value)}
						/>
					</FormFields>
				</FormSection>

				<FormFooter>
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>

					<Button type="submit" disabled={isLoading}>
						<Spinner loading={isLoading} />
						Save
					</Button>
				</FormFooter>
			</HorizontalForm>
		</>
	);
};
