import { useFormik } from "formik";
import { ArrowLeftIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import { hasApiFieldErrors, isApiError } from "#/api/errors";
import type { UpdateUserProfileRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormFields, FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { docs } from "#/utils/docs";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: displayNameValidator("Name"),
	avatar_url: Yup.string(),
});

interface EditUserFormProps {
	error?: unknown;
	isLoading: boolean;
	initialValues: UpdateUserProfileRequest;
	/** Hidden for login types whose avatar is synced from an identity provider. */
	canEditAvatar: boolean;
	headerActions?: ReactNode;
	onSubmit: (values: UpdateUserProfileRequest) => void;
	onCancel: () => void;
}

export const EditUserForm: FC<EditUserFormProps> = ({
	error,
	isLoading,
	initialValues,
	canEditAvatar,
	headerActions,
	onSubmit,
	onCancel,
}) => {
	// `enableReinitialize` is intentionally omitted: the user query is
	// invalidated by the actions in the header, and reinitializing would
	// discard whatever the admin has typed when a refetch lands.
	const form = useFormik<UpdateUserProfileRequest>({
		initialValues,
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers(form, error);
	// Read from the saved user rather than the draft so the heading does not
	// change on every keystroke.
	const heading = initialValues.name.trim() || initialValues.username;

	return (
		<div>
			<div className="flex items-center justify-between">
				<Button variant="subtle" asChild className="-ml-3">
					<Link to="/deployment/users">
						<ArrowLeftIcon />
						<span>Back to users</span>
					</Link>
				</Button>
				{headerActions}
			</div>

			<div className="pt-6">
				<SettingsHeader>
					<SettingsHeaderTitle>Edit {heading}</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Change how this user appears across Coder.{" "}
						<SettingsHeaderDocsLink
							href={docs("/admin/users#edit-a-users-profile")}
						/>
					</SettingsHeaderDescription>
				</SettingsHeader>

				<div className="border border-solid p-6 rounded-lg">
					<form
						onSubmit={form.handleSubmit}
						autoComplete="off"
						className="flex flex-col gap-6"
					>
						{isApiError(error) && !hasApiFieldErrors(error) && (
							<ErrorAlert error={error} />
						)}

						<FormFields>
							<FormField
								field={getFieldHelpers("username", {
									helperText: "Unique identifier.",
								})}
								label="Username"
								required
								onChange={onChangeTrimmed(form)}
								autoComplete="username"
								autoFocus
							/>

							<FormField
								field={getFieldHelpers("name", {
									helperText:
										"Friendly name. Defaults to the username if blank.",
								})}
								label="Name"
								autoComplete="name"
							/>

							{canEditAvatar && (
								<IconField
									{...getFieldHelpers("avatar_url", {
										helperText: "URL or emoji shown for this user.",
									})}
									label="Avatar URL"
									onChange={onChangeTrimmed(form)}
									onPickEmoji={(value) =>
										form.setFieldValue("avatar_url", value)
									}
								/>
							)}
						</FormFields>

						<FormFooter className="mt-0">
							<Button onClick={onCancel} variant="outline">
								Cancel
							</Button>
							<Button type="submit" disabled={isLoading}>
								<Spinner loading={isLoading} />
								Save
							</Button>
						</FormFooter>
					</form>
				</div>
			</div>
		</div>
	);
};
