import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type { CreateGroupRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { useFormik } from "formik";
import type { FC } from "react";
import { useNavigate } from "react-router-dom";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import * as Yup from "yup";

const validationSchema = Yup.object({
	name: Yup.string().required().label("Name"),
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
			<SettingsHeader
				title="New Group"
				description="Create a group in this organization."
			/>

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
						/>
						<TextField
							{...getFieldHelpers("display_name", {
								helperText: "Optional: keep empty to default to the name.",
							})}
							fullWidth
							label="Display Name"
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
				<FormFooter onCancel={onCancel} isLoading={isLoading} />
			</HorizontalForm>
		</>
	);
};
export default CreateGroupPageView;
