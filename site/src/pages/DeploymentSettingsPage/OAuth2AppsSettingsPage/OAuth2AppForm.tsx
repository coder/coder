import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import {
	getFormHelpers,
	iconValidator,
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
	onSubmit: (data: OAuth2AppFormValues) => void | Promise<void>;
	error?: unknown;
	isUpdating: boolean;
	defaultValues?: OAuth2AppFormValues;
	disabled: boolean;
	onIconChange?: (icon: string) => void;
};

const BACK_HREF = "/deployment/oauth2-provider/apps";

const isHttpUrl = (value: string | undefined): boolean => {
	if (!value) {
		return false;
	}
	try {
		const url = new URL(value);
		return url.protocol === "http:" || url.protocol === "https:";
	} catch {
		return false;
	}
};

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	callback_url: Yup.string()
		.trim()
		.required("Please enter a callback URL.")
		.test("http-url", "Callback URL must be a valid URL.", (value) =>
			isHttpUrl(value),
		),
	icon: iconValidator,
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
	const didSubmit = useRef(false);
	const form = useFormik<OAuth2AppFormValues>({
		initialValues: {
			name: app?.name ?? defaultValues?.name ?? "",
			callback_url: app?.callback_url ?? defaultValues?.callback_url ?? "",
			icon: app?.icon ?? defaultValues?.icon ?? "",
		},
		validationSchema,
		validateOnMount: true,
		onSubmit: async (values) => {
			didSubmit.current = true;
			await onSubmit(values);
		},
	});
	const getFieldHelpers = getFormHelpers(form, error);
	const iconField = getFieldHelpers("icon");
	const formDisabled = disabled || isUpdating;
	const editing = Boolean(app);
	const submitDisabled =
		formDisabled || !form.isValid || (editing && !form.dirty);

	// When the parent's mutation finishes without an error, treat the just-
	// submitted values as the new baseline so the unsaved-changes prompt does
	// not fire on subsequent navigations.
	const previousIsUpdating = useRef(isUpdating);
	useEffect(() => {
		if (previousIsUpdating.current && !isUpdating) {
			if (didSubmit.current && !error) {
				form.resetForm({ values: form.values });
			}
			didSubmit.current = false;
		}
		previousIsUpdating.current = isUpdating;
	}, [isUpdating, error, form]);

	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);

	const handleIconChange = (value: string) => {
		void form.setFieldValue("icon", value);
		void form.setFieldTouched("icon", true);
		onIconChange?.(value);
	};

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(error) && <ErrorAlert error={error} />}
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
					<IconField
						id="icon"
						value={form.values.icon}
						disabled={formDisabled}
						label={null}
						onChange={(event) => handleIconChange(event.target.value)}
						onPickEmoji={handleIconChange}
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
			</FormFields>
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={unsavedChanges.isOpen}
				onClose={unsavedChanges.onCancel}
				onConfirm={unsavedChanges.onConfirm}
				title="Unsaved changes"
				confirmText="Confirm"
				description={
					<div className="flex items-start gap-3">
						<TriangleAlertIcon className="size-icon-sm mt-1 shrink-0" />
						<p className="m-0">
							Your updates haven't been saved. Leave anyway?
						</p>
					</div>
				}
			/>
		</Form>
	);
};
