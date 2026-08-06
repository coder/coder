import { type FormikContextType, useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { PasswordField } from "#/components/PasswordField/PasswordField";
import { Spinner } from "#/components/Spinner/Spinner";
import { getFormHelpers } from "#/utils/formUtils";

interface SecurityFormValues {
	old_password: string;
	password: string;
	confirm_password: string;
}

const validationSchema = Yup.object({
	old_password: Yup.string().trim().required("Old password is required"),
	password: Yup.string().trim().required("New password is required"),
	confirm_password: Yup.string()
		.trim()
		.test(
			"passwords-match",
			"Password and confirmation must match",
			function (value) {
				return (this.parent as SecurityFormValues).password === value;
			},
		),
});

interface SecurityFormProps {
	disabled: boolean;
	isLoading: boolean;
	onSubmit: (values: SecurityFormValues) => void;
	error?: unknown;
}

export const SecurityForm: FC<SecurityFormProps> = ({
	disabled,
	isLoading,
	onSubmit,
	error,
}) => {
	const form: FormikContextType<SecurityFormValues> =
		useFormik<SecurityFormValues>({
			initialValues: {
				old_password: "",
				password: "",
				confirm_password: "",
			},
			validationSchema,
			onSubmit,
		});
	const getFieldHelpers = getFormHelpers<SecurityFormValues>(form, error);

	if (disabled) {
		return (
			<Alert severity="info">
				Password changes are only allowed for password based accounts.
			</Alert>
		);
	}

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(error) && <ErrorAlert error={error} />}
				<FormField
					field={getFieldHelpers("old_password")}
					label="Old Password"
					type="password"
					autoComplete="current-password"
				/>
				<PasswordField
					field={getFieldHelpers("password")}
					label="New Password"
					autoComplete="new-password"
				/>
				<FormField
					field={getFieldHelpers("confirm_password")}
					label="Confirm Password"
					type="password"
					autoComplete="new-password"
				/>

				<div>
					<Button disabled={isLoading} type="submit">
						<Spinner loading={isLoading} />
						Update password
					</Button>
				</div>
			</FormFields>
		</Form>
	);
};
