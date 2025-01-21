import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import type { CreateGroupRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { FormFooter } from "components/Form/Form";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { IconField } from "components/IconField/IconField";
import { Margins } from "components/Margins/Margins";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { type FormikTouched, useFormik } from "formik";
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
	// Helpful to show field errors on Storybook
	initialTouched?: FormikTouched<CreateGroupRequest>;
};

export const CreateGroupPageView: FC<CreateGroupPageViewProps> = ({
	onSubmit,
	error,
	isLoading,
	initialTouched,
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
		initialTouched,
	});
	const getFieldHelpers = getFormHelpers<CreateGroupRequest>(form, error);
	const onCancel = () => navigate("/deployment/groups");

	return (
		<Margins>
			<FullPageForm title="Create group">
				<form onSubmit={form.handleSubmit}>
					<Stack spacing={2.5}>
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
					</Stack>

					<FormFooter className="mt-8">
						<Button onClick={onCancel} variant="outline">
							Cancel
						</Button>

						<Button type="submit" disabled={isLoading}>
							{isLoading && <Spinner />}
							Save
						</Button>
					</FormFooter>
				</form>
			</FullPageForm>
		</Margins>
	);
};
export default CreateGroupPageView;
