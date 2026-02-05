import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Field,
	FieldError,
	FieldGroup,
	FieldLabel,
	FieldSet,
} from "components/Field/Field";
import { Form, FormFields } from "components/Form/Form";
import { Input } from "components/Input/Input";
import { Spinner } from "components/Spinner/Spinner";
import { type FormikContextType, useFormik } from "formik";
import type { FC } from "react";
import { getFieldHelpers } from "utils/formUtils";
import * as Yup from "yup";

interface SecurityFormValues {
	old_password: string;
	password: string;
	confirm_password: string;
}

export const Language = {
	oldPasswordLabel: "Old Password",
	newPasswordLabel: "New Password",
	confirmPasswordLabel: "Confirm Password",
	oldPasswordRequired: "Old password is required",
	newPasswordRequired: "New password is required",
	confirmPasswordRequired: "Password confirmation is required",
	passwordMinLength: "Password must be at least 8 characters",
	passwordMaxLength: "Password must be no more than 64 characters",
	confirmPasswordMatch: "Password and confirmation must match",
	updatePassword: "Update password",
};

const validationSchema = Yup.object({
	old_password: Yup.string().trim().required(Language.oldPasswordRequired),
	password: Yup.string().trim().required(Language.newPasswordRequired),
	confirm_password: Yup.string()
		.trim()
		.test("passwords-match", Language.confirmPasswordMatch, function (value) {
			return (this.parent as SecurityFormValues).password === value;
		}),
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
	const fieldHelper = getFieldHelpers(form);

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

				<FieldSet>
					<FieldGroup>
						<Field>
							<FieldLabel htmlFor="old_password">Old Password</FieldLabel>
							<Input
								{...fieldHelper("old_password")}
								type="password"
								disabled={isLoading}
							/>
							<FieldError>{form.errors.old_password}</FieldError>
						</Field>
						<Field>
							<FieldLabel htmlFor="password">New Password</FieldLabel>
							<Input
								{...fieldHelper("password")}
								type="password"
								disabled={isLoading}
							/>
							<FieldError>{form.errors.password}</FieldError>
						</Field>
						<Field>
							<FieldLabel htmlFor="confirm_password">
								Confirm Password
							</FieldLabel>
							<Input
								{...fieldHelper("confirm_password")}
								type="password"
								disabled={isLoading}
							/>
							<FieldError>{form.errors.confirm_password}</FieldError>
						</Field>
					</FieldGroup>
				</FieldSet>

				<div>
					<Button disabled={isLoading} type="submit">
						<Spinner loading={isLoading} />
						{Language.updatePassword}
					</Button>
				</div>
			</FormFields>
		</Form>
	);
};
