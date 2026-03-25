import { useFormik } from "formik";
import type { FC } from "react";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import { hasApiFieldErrors, isApiError } from "#/api/errors";
import type { UpdateUserProfileRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { FullPageForm } from "#/components/FullPageForm/FullPageForm";
import { Spinner } from "#/components/Spinner/Spinner";

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: displayNameValidator("Full name"),
});

interface EditUserFormProps {
	error?: unknown;
	isLoading: boolean;
	initialValues: UpdateUserProfileRequest;
	onSubmit: (values: UpdateUserProfileRequest) => void;
	onCancel: () => void;
}

export const EditUserForm: FC<EditUserFormProps> = ({
	error,
	isLoading,
	initialValues,
	onSubmit,
	onCancel,
}) => {
	const form = useFormik<UpdateUserProfileRequest>({
		initialValues,
		validationSchema,
		onSubmit,
		enableReinitialize: true,
	});

	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<FullPageForm title="Edit user">
			{isApiError(error) && !hasApiFieldErrors(error) && (
				<ErrorAlert error={error} className="mb-8" />
			)}
			<form onSubmit={form.handleSubmit} autoComplete="off">
				<div className="flex flex-col gap-6">
					<FormField
						field={getFieldHelpers("username")}
						label="Username"
						id="username"
						name="username"
						value={form.values.username}
						onChange={onChangeTrimmed(form)}
						onBlur={form.handleBlur}
						autoComplete="username"
						autoFocus
					/>

					<FormField
						field={getFieldHelpers("name")}
						label={
							<>
								Full name{" "}
								<span className="font-normal text-content-secondary">
									(optional)
								</span>
							</>
						}
						id="name"
						name="name"
						value={form.values.name}
						onChange={form.handleChange}
						onBlur={form.handleBlur}
						autoComplete="name"
					/>
				</div>

				<FormFooter className="mt-8">
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>
					<Button type="submit" disabled={isLoading}>
						<Spinner loading={isLoading} />
						Save
					</Button>
				</FormFooter>
			</form>
		</FullPageForm>
	);
};
