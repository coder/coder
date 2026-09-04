import { useFormik } from "formik";
import { ArrowLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import type { CreateGroupRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormFields, FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
});

type CreateGroupPageViewProps = {
	onSubmit: (data: CreateGroupRequest) => void;
	onCancel: () => void;
	error?: unknown;
	isLoading: boolean;
	showOrganizations: boolean;
};

export const CreateGroupPageView: FC<CreateGroupPageViewProps> = ({
	onSubmit,
	onCancel,
	error,
	isLoading,
	showOrganizations,
}) => {
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

	return (
		<>
			<Button variant="subtle" asChild className="-ml-3">
				<Link to="..">
					<ArrowLeftIcon />
					<span>Back to groups</span>
				</Link>
			</Button>

			<div className="pt-6">
				<SettingsHeader>
					<SettingsHeaderTitle>New group</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Add a group to this{" "}
						{showOrganizations ? "organization" : "deployment"}.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<form
					className="flex flex-col gap-6 border border-solid p-6 rounded-lg"
					onSubmit={form.handleSubmit}
				>
					{Boolean(error) && !isApiValidationError(error) && (
						<ErrorAlert error={error} />
					)}

					<FormFields>
						<FormField
							field={getFieldHelpers("name", {
								helperText: "Unique identifier.",
							})}
							label="Name"
							onChange={onChangeTrimmed(form)}
							autoFocus
							autoComplete="name"
							required
						/>
						<FormField
							field={getFieldHelpers("display_name", {
								helperText: "Friendly name. Defaults to the name if blank.",
							})}
							label="Display name"
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

					<FormFooter className="mt-0">
						<Button type="button" onClick={onCancel} variant="outline">
							Cancel
						</Button>
						<Button type="submit" disabled={isLoading}>
							<Spinner loading={isLoading} />
							Save
						</Button>
					</FormFooter>
				</form>
			</div>
		</>
	);
};
