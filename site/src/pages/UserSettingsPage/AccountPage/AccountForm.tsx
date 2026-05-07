import { type FormikTouched, useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type { UpdateUserProfileRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: Yup.string(),
});

interface AccountFormProps {
	editable: boolean;
	email: string;
	isLoading: boolean;
	initialValues: UpdateUserProfileRequest;
	onSubmit: (values: UpdateUserProfileRequest) => void;
	updateProfileError?: unknown;
	// initialTouched is only used for testing the error state of the form.
	initialTouched?: FormikTouched<UpdateUserProfileRequest>;
}

export const AccountForm: FC<AccountFormProps> = ({
	editable,
	email,
	isLoading,
	onSubmit,
	initialValues,
	updateProfileError,
	initialTouched,
}) => {
	const form = useFormik({
		initialValues,
		validationSchema,
		onSubmit,
		initialTouched,
	});
	const getFieldHelpers = getFormHelpers(form, updateProfileError);

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(updateProfileError) && (
					<ErrorAlert error={updateProfileError} />
				)}

				<FormField
					field={getFieldHelpers("email")}
					label="Email"
					value={email}
					disabled
				/>
				<FormField
					field={getFieldHelpers("username")}
					onChange={onChangeTrimmed(form)}
					aria-disabled={!editable}
					autoComplete="username"
					disabled={!editable}
					className="w-full"
					label="Username"
				/>
				<FormField
					field={{
						...getFieldHelpers("name"),
						helperText:
							'The human-readable name is optional and can be accessed in a template via the "data.coder_workspace_owner.me.full_name" property.',
					}}
					autoComplete="name"
					className="w-full"
					label="Name"
					onBlur={(event) => {
						event.target.value = event.target.value.trim();
						form.handleChange(event);
					}}
				/>
				<div>
					<Button disabled={isLoading} type="submit">
						<Spinner loading={isLoading} />
						Update account
					</Button>
				</div>
			</FormFields>
		</Form>
	);
};
