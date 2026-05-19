import { type FormikTouched, useFormik } from "formik";
import type { FC } from "react";
import type {
	CreateUserSecretRequest,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import { cn } from "#/utils/cn";
import { getFormHelpers } from "#/utils/formUtils";
import {
	buildCreateUserSecretRequest,
	buildUpdateUserSecretRequest,
	getCreateSecretRequiredFieldErrors,
	mapSecretApiErrorToFormErrors,
	type SecretFieldErrors,
	type SecretFormValues,
} from "./secretForm";

type SecretDialogProps = {
	open: boolean;
	secret?: UserSecret;
	isSubmitting: boolean;
	returnFocusElement?: HTMLElement | null;
	onClose: () => void;
	onCreateSecret: (
		request: CreateUserSecretRequest,
	) => Promise<unknown> | unknown;
	onUpdateSecret: (
		name: string,
		request: UpdateUserSecretRequest,
	) => Promise<unknown> | unknown;
};

const emptyValues: SecretFormValues = {
	name: "",
	value: "",
	description: "",
	env_name: "",
	file_path: "",
};

const infoText =
	"Secret values cannot be retrieved once saved. Save any value you add or update in a secure location.";

export const SecretDialog: FC<SecretDialogProps> = ({
	open,
	secret,
	isSubmitting,
	returnFocusElement,
	onClose,
	onCreateSecret,
	onUpdateSecret,
}) => {
	const isEdit = Boolean(secret);
	const initialValues = secret
		? {
				name: secret.name,
				value: "",
				description: secret.description,
				env_name: secret.env_name,
				file_path: secret.file_path,
			}
		: emptyValues;

	const form = useFormik<SecretFormValues>({
		initialValues,
		enableReinitialize: true,
		validateOnMount: true,
		validate: (values) =>
			isEdit ? {} : getCreateSecretRequiredFieldErrors(values),
		onSubmit: async (values, helpers) => {
			helpers.setStatus(undefined);
			try {
				if (secret) {
					const request = buildUpdateUserSecretRequest(secret, values);
					await onUpdateSecret(secret.name, request);
				} else {
					await onCreateSecret(buildCreateUserSecretRequest(values));
				}
				helpers.resetForm();
				onClose();
			} catch (error) {
				const formErrors = mapSecretApiErrorToFormErrors(error);
				helpers.setErrors(formErrors.fieldErrors);
				helpers.setTouched(
					touchedFromFieldErrors(formErrors.fieldErrors),
					false,
				);
				helpers.setStatus(formErrors.formError);
			}
		},
	});

	const request = secret
		? buildUpdateUserSecretRequest(secret, form.values)
		: undefined;
	const hasUpdate = request ? Object.keys(request).length > 0 : false;
	const isBusy = isSubmitting || form.isSubmitting;
	const confirmDisabled =
		isBusy || !form.isValid || (secret ? !hasUpdate : !form.dirty);
	const getFieldHelpers = getFormHelpers(form);
	const formError = form.status as string | undefined;

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (!nextOpen && !isBusy) {
					form.resetForm();
					onClose();
				}
			}}
		>
			<DialogContent
				className="max-h-[90vh] overflow-y-auto"
				aria-describedby={undefined}
				onCloseAutoFocus={(event) => {
					if (returnFocusElement?.isConnected) {
						event.preventDefault();
						returnFocusElement.focus();
					}
				}}
			>
				<DialogHeader>
					<DialogTitle>{secret ? "Edit secret" : "Add secret"}</DialogTitle>
				</DialogHeader>

				<form onSubmit={form.handleSubmit} className="flex flex-col gap-5">
					<Alert severity="info" className="text-content-secondary">
						<AlertDescription>{infoText}</AlertDescription>
					</Alert>

					{formError && (
						<Alert severity="error" prominent>
							<AlertDescription>{formError}</AlertDescription>
						</Alert>
					)}

					{secret ? (
						<>
							<SecretFields
								getFieldHelpers={getFieldHelpers}
								disableName
								showValue={false}
							/>
							<FormField
								field={getFieldHelpers("value")}
								label="Value"
								type="password"
								placeholder="enter secret value"
								autoComplete="off"
							/>
							<SecretDescriptionField field={getFieldHelpers("description")} />
						</>
					) : (
						<>
							<SecretFields getFieldHelpers={getFieldHelpers} showValue />
							<SecretDescriptionField field={getFieldHelpers("description")} />
						</>
					)}

					<DialogFooter>
						<Button
							variant="outline"
							disabled={isBusy}
							onClick={() => {
								form.resetForm();
								onClose();
							}}
						>
							Cancel
						</Button>
						<Button type="submit" disabled={confirmDisabled}>
							<Spinner loading={isSubmitting || form.isSubmitting} />
							{secret ? "Update" : "Save"}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	);
};

type SecretFieldsProps = {
	getFieldHelpers: ReturnType<typeof getFormHelpers<SecretFormValues>>;
	disableName?: boolean;
	showValue: boolean;
};

const SecretFields: FC<SecretFieldsProps> = ({
	getFieldHelpers,
	disableName,
	showValue,
}) => {
	return (
		<>
			<FormField
				field={getFieldHelpers("name")}
				label="Name"
				placeholder="enter name"
				disabled={disableName}
			/>
			<FormField
				field={getFieldHelpers("env_name", {
					helperText: "Use the same name as the variable in your template.",
				})}
				label="Env var"
				placeholder="VARIABLE_NAME"
			/>
			<FormField
				field={getFieldHelpers("file_path", {
					helperText: "File paths must start with ~/ or /.",
				})}
				label="File path"
				placeholder="/usr/local/secrets"
			/>
			{showValue && (
				<FormField
					field={getFieldHelpers("value")}
					label="Value"
					type="password"
					placeholder="enter secret value"
					autoComplete="off"
				/>
			)}
		</>
	);
};

type SecretDescriptionFieldProps = {
	field: ReturnType<ReturnType<typeof getFormHelpers<SecretFormValues>>>;
};

const SecretDescriptionField: FC<SecretDescriptionFieldProps> = ({ field }) => {
	const errorId = `${field.id}-error`;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={field.id}>Description</Label>
			<Textarea
				id={field.id}
				name={field.name}
				value={field.value}
				onChange={field.onChange}
				onBlur={field.onBlur}
				placeholder="Optional"
				aria-invalid={field.error}
				aria-describedby={field.error ? errorId : undefined}
				className={cn(field.error && "border-border-destructive")}
			/>
			{field.error && (
				<span id={errorId} className="text-xs text-content-destructive">
					{field.helperText}
				</span>
			)}
		</div>
	);
};

function touchedFromFieldErrors(
	fieldErrors: SecretFieldErrors,
): FormikTouched<SecretFormValues> {
	return Object.fromEntries(
		Object.keys(fieldErrors).map((field) => [field, true]),
	) as FormikTouched<SecretFormValues>;
}
