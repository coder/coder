import { type FormikTouched, useFormik } from "formik";
import { type FC, type ReactNode, useState } from "react";
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
import { Input } from "#/components/Input/Input";
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
	) => Promise<UserSecret> | UserSecret;
	onUpdateSecret: (
		name: string,
		request: UpdateUserSecretRequest,
	) => Promise<UserSecret> | UserSecret;
};

const emptyValues: SecretFormValues = {
	name: "",
	value: "",
	description: "",
	env_name: "",
	file_path: "",
};

const infoText = "Secret values cannot be retrieved once saved.";
export const SAVED_SECRET_VALUE_DISPLAY = "••••••••••••••••••••";

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
	const [clearValueRequested, setClearValueRequested] = useState(false);

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
					const request = buildUpdateUserSecretRequest(secret, values, {
						clearValue: clearValueRequested,
					});
					await onUpdateSecret(secret.name, request);
				} else {
					await onCreateSecret(buildCreateUserSecretRequest(values));
				}
				setClearValueRequested(false);
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

	const closeDialog = () => {
		setClearValueRequested(false);
		form.resetForm();
		onClose();
	};

	const request = secret
		? buildUpdateUserSecretRequest(secret, form.values, {
				clearValue: clearValueRequested,
			})
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
					closeDialog();
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

				<form
					onSubmit={form.handleSubmit}
					className="flex flex-col gap-5"
					autoComplete="off"
				>
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
							<SecretValueField
								key={`${secret.name}-${open}`}
								field={getFieldHelpers("value", {
									helperText: "Leave blank to keep the existing value.",
								})}
								placeholder="Leave blank to keep existing value"
								showSavedValue={open}
								clearValueRequested={clearValueRequested}
								onClearValue={() => {
									setClearValueRequested(true);
									void form.setFieldValue("value", "", false);
								}}
								onUndoClearValue={() => {
									setClearValueRequested(false);
									void form.setFieldValue("value", "", false);
								}}
							/>
							<SecretDescriptionField field={getFieldHelpers("description")} />
						</>
					) : (
						<>
							<SecretFields
								getFieldHelpers={getFieldHelpers}
								showRequiredLabels
								showValue
							/>
							<SecretDescriptionField field={getFieldHelpers("description")} />
						</>
					)}

					<DialogFooter>
						<Button variant="outline" disabled={isBusy} onClick={closeDialog}>
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
	showRequiredLabels?: boolean;
	showValue: boolean;
};

const SecretFields: FC<SecretFieldsProps> = ({
	getFieldHelpers,
	disableName,
	showRequiredLabels,
	showValue,
}) => {
	return (
		<>
			<FormField
				field={getFieldHelpers("name", {
					helperText: disableName
						? "Unique identifier (can’t be changed)."
						: undefined,
				})}
				label={
					showRequiredLabels ? (
						<RequiredFieldLabel>Name</RequiredFieldLabel>
					) : (
						"Name"
					)
				}
				placeholder="Secret name"
				autoComplete="off"
				className="placeholder:text-content-disabled"
				disabled={disableName}
				aria-required={showRequiredLabels}
				data-lpignore="true"
				data-1p-ignore="true"
				data-form-type="other"
			/>
			<FormField
				field={getFieldHelpers("env_name", {
					helperText:
						"Optional. Exposes the secret as an environment variable with this name in your workspace.",
				})}
				label="Environment variable"
				placeholder="SERVICE_TOKEN"
				autoComplete="off"
				className="placeholder:text-content-disabled"
				data-lpignore="true"
				data-1p-ignore="true"
				data-form-type="other"
			/>
			<FormField
				field={getFieldHelpers("file_path", {
					helperText:
						"Optional. Exposes the secret as a file at this path in your workspace. Path must start with ~/ or /.",
				})}
				label="File path"
				placeholder="~/api-key.txt"
				autoComplete="off"
				className="placeholder:text-content-disabled"
				data-lpignore="true"
				data-1p-ignore="true"
				data-form-type="other"
			/>
			{showValue && (
				<SecretValueField
					field={getFieldHelpers("value")}
					placeholder="Enter secret value"
					required={showRequiredLabels}
				/>
			)}
		</>
	);
};

type RequiredFieldLabelProps = {
	children: ReactNode;
};

const RequiredFieldLabel: FC<RequiredFieldLabelProps> = ({ children }) => {
	return (
		<span className="after:ml-1 after:text-content-destructive after:content-['*']">
			{children}
		</span>
	);
};

type SecretValueFieldProps = {
	field: ReturnType<ReturnType<typeof getFormHelpers<SecretFormValues>>>;
	placeholder: string;
	required?: boolean;
	showSavedValue?: boolean;
	clearValueRequested?: boolean;
	onClearValue?: () => void;
	onUndoClearValue?: () => void;
};

const SecretValueField: FC<SecretValueFieldProps> = ({
	field,
	placeholder,
	required,
	showSavedValue = false,
	clearValueRequested = false,
	onClearValue,
	onUndoClearValue,
}) => {
	const [hasHiddenSavedValue, setHasHiddenSavedValue] = useState(false);
	const isShowingSavedValue =
		showSavedValue && !clearValueRequested && !hasHiddenSavedValue;

	const value = clearValueRequested
		? ""
		: isShowingSavedValue
			? SAVED_SECRET_VALUE_DISPLAY
			: field.value;
	const maskTypedValue =
		!clearValueRequested &&
		!isShowingSavedValue &&
		typeof field.value === "string" &&
		field.value !== "";
	const displayField = clearValueRequested
		? {
				...field,
				helperText: field.error
					? field.helperText
					: "Saved value will be cleared when you update.",
			}
		: field;
	const errorId = `${field.id}-error`;
	const helperId = `${field.id}-helper`;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={field.id}>
				{required ? <RequiredFieldLabel>Value</RequiredFieldLabel> : "Value"}
			</Label>
			<div className="flex flex-col gap-2 sm:flex-row sm:items-start">
				<Input
					id={field.id}
					name={field.name}
					type="text"
					value={value}
					placeholder={placeholder}
					autoComplete="off"
					aria-required={required}
					aria-invalid={displayField.error}
					aria-describedby={
						displayField.error
							? errorId
							: displayField.helperText
								? helperId
								: undefined
					}
					disabled={clearValueRequested}
					className={cn(
						"placeholder:text-content-disabled sm:flex-1",
						displayField.error && "border-border-destructive",
						maskTypedValue && "[-webkit-text-security:circle]",
					)}
					onFocus={(event) => {
						if (isShowingSavedValue) {
							event.currentTarget.value = "";
							setHasHiddenSavedValue(true);
						}
					}}
					onChange={(event) => {
						if (isShowingSavedValue) {
							setHasHiddenSavedValue(true);
						}
						field.onChange(event);
					}}
					onBlur={(event) => {
						field.onBlur(event);
						if (showSavedValue && event.currentTarget.value === "") {
							setHasHiddenSavedValue(false);
						}
					}}
				/>
				{onClearValue && onUndoClearValue && (
					<Button
						type="button"
						variant="outline"
						size="sm"
						className={cn(
							"h-10 w-16 shrink-0",
							!clearValueRequested &&
								"text-content-secondary hover:border-border-destructive hover:text-content-destructive",
						)}
						onClick={clearValueRequested ? onUndoClearValue : onClearValue}
					>
						{clearValueRequested ? "Undo" : "Clear"}
					</Button>
				)}
			</div>
			{displayField.error ? (
				<span id={errorId} className="text-xs text-content-destructive">
					{displayField.helperText}
				</span>
			) : (
				displayField.helperText && (
					<span id={helperId} className="text-xs text-content-secondary">
						{displayField.helperText}
					</span>
				)
			)}
		</div>
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
				className={cn(
					"placeholder:text-content-disabled",
					field.error && "border-border-destructive",
				)}
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
