import { useFormik } from "formik";
import type { FC } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { IconPickerField } from "#/pages/AISettingsPage/MCPServersPage/components/IconPickerField";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

type OAuth2AppFormValues = {
	name: string;
	callback_url: string;
	icon: string;
};

type OAuth2AppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: OAuth2AppFormValues) => void;
	error?: unknown;
	isUpdating: boolean;
	defaultValues?: OAuth2AppFormValues;
	disabled: boolean;
	onIconChange?: (icon: string) => void;
};

const BACK_HREF = "/deployment/oauth2-provider/apps";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	callback_url: Yup.string()
		.trim()
		.required("Please enter a callback URL.")
		.url("Callback URL must be a valid URL."),
	icon: Yup.string(),
});

export const OAuth2AppForm: FC<OAuth2AppFormProps> = ({
	app,
	onSubmit,
	error,
	isUpdating,
	defaultValues,
	disabled,
	onIconChange,
}) => {
	const form = useFormik<OAuth2AppFormValues>({
		initialValues: {
			name: app?.name ?? defaultValues?.name ?? "",
			callback_url: app?.callback_url ?? defaultValues?.callback_url ?? "",
			icon: app?.icon ?? defaultValues?.icon ?? "",
		},
		validationSchema,
		validateOnMount: true,
		onSubmit: (values) => {
			onSubmit(values);
		},
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const iconField = getFieldHelpers("icon");
	const formDisabled = disabled || isUpdating;
	const editing = Boolean(app);
	const submitDisabled =
		formDisabled || !form.isValid || (editing && !form.dirty);

	return (
		<form onSubmit={form.handleSubmit}>
			<div className="flex flex-col gap-5">
				<FormField
					field={getFieldHelpers("name")}
					label="Name"
					description="The name of your Coder app."
					disabled={formDisabled}
					onChange={onChangeTrimmed(form)}
					autoFocus
					required
				/>
				<FormField
					field={getFieldHelpers("callback_url")}
					label="Callback URL"
					description="The full URL to redirect to after a user authorizes an installation."
					disabled={formDisabled}
					required
				/>
				<div className="flex flex-col gap-2">
					<Label htmlFor="icon">Icon</Label>
					<div className="text-xs text-content-secondary">
						Optional. URL or emoji shown for this application.
					</div>
					<IconPickerField
						id="icon"
						value={form.values.icon}
						disabled={formDisabled}
						onChange={(value) => {
							void form.setFieldValue("icon", value);
							void form.setFieldTouched("icon", true);
							onIconChange?.(value);
						}}
					/>
					{iconField.error ? (
						<span className="text-xs text-content-destructive">
							{iconField.helperText}
						</span>
					) : (
						iconField.helperText && (
							<span className="text-xs text-content-secondary">
								{iconField.helperText}
							</span>
						)
					)}
				</div>

				<div className="flex justify-end gap-4">
					<Button variant="outline" asChild>
						<Link to={BACK_HREF}>Cancel</Link>
					</Button>
					<Button disabled={submitDisabled} type="submit">
						<Spinner loading={isUpdating} />
						{app ? "Update application" : "Create application"}
					</Button>
				</div>
			</div>
		</form>
	);
};
