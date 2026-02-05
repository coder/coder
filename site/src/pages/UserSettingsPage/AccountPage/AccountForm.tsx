import type { UpdateUserProfileRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Field,
	FieldDescription,
	FieldError,
	FieldGroup,
	FieldLabel,
	FieldSet,
} from "components/Field/Field";
import { Form, FormFields } from "components/Form/Form";
import { Input } from "components/Input/Input";
import { Spinner } from "components/Spinner/Spinner";
import { type FormikTouched, useFormik } from "formik";
import type { FC } from "react";
import {
	getFieldHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

export const Language = {
	usernameLabel: "Username",
	emailLabel: "Email",
	nameLabel: "Name",
	updateSettings: "Update account",
};

const validationSchema = Yup.object({
	username: nameValidator(Language.usernameLabel),
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
	const fieldHelper = getFieldHelpers(form);

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(updateProfileError) && (
					<ErrorAlert error={updateProfileError} />
				)}

				<FieldSet>
					<FieldGroup>
						<Field>
							<FieldLabel htmlFor="email">Email</FieldLabel>
							<Input
								{...fieldHelper("email")}
								value={email}
								autoComplete="none"
								disabled
							/>
						</Field>
						<Field>
							<FieldLabel htmlFor="username">Username</FieldLabel>
							<Input
								{...fieldHelper("username")}
								onChange={onChangeTrimmed(form)}
								aria-disabled={!editable}
								autoComplete="none"
								disabled={!editable}
							/>
							<FieldError>{form.errors.username}</FieldError>
						</Field>
						<Field>
							<FieldLabel htmlFor="name">Name</FieldLabel>
							<Input
								{...fieldHelper("name")}
								onBlur={(e) => {
									e.target.value = e.target.value.trim();
									form.handleChange(e);
								}}
							/>
							<FieldError>{form.errors.name}</FieldError>
							<FieldDescription>
								The human-readable name is optional and can be accessed in a
								template via the "data.coder_workspace_owner.me.full_name"
								property.
							</FieldDescription>
						</Field>
					</FieldGroup>
				</FieldSet>

				<div>
					<Button disabled={isLoading} type="submit">
						<Spinner loading={isLoading} />
						{Language.updateSettings}
					</Button>
				</div>
			</FormFields>
		</Form>
	);
};
