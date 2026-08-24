import { type FormikTouched, useFormik } from "formik";
import { type FC, type ReactNode, useState } from "react";
import {
	type FieldError,
	getErrorMessage,
	isApiError,
	isApiErrorResponse,
} from "#/api/errors";
import {
	type CreateUserSecretRequest,
	type ImportUserSecretsRequest,
	MaxSecretsFileBytes,
	type UpdateUserSecretRequest,
	type UserSecret,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { FileUpload } from "#/components/FileUpload/FileUpload";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Separator } from "#/components/Separator/Separator";
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
	secretsFileFormatFromFilename,
} from "./secretForm";

type SecretDialogProps = {
	open: boolean;
	secret?: UserSecret;
	filePathEnabled: boolean;
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
	onImportSecrets: (request: ImportUserSecretsRequest) => Promise<UserSecret[]>;
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
	filePathEnabled,
	isSubmitting,
	returnFocusElement,
	onClose,
	onCreateSecret,
	onUpdateSecret,
	onImportSecrets,
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
	const [importFile, setImportFile] = useState<File | undefined>(undefined);
	const [isImporting, setIsImporting] = useState(false);
	const [importError, setImportError] = useState<unknown>(undefined);

	const form = useFormik<SecretFormValues>({
		initialValues,
		enableReinitialize: true,
		validateOnMount: true,
		validate: (values) =>
			isEdit ? {} : getCreateSecretRequiredFieldErrors(values, filePathEnabled),
		onSubmit: async (values, helpers) => {
			helpers.setStatus(undefined);
			try {
				if (secret) {
					const request = buildUpdateUserSecretRequest(secret, values, {
						clearValue: clearValueRequested,
						filePathEnabled,
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
		setImportFile(undefined);
		setImportError(undefined);
		setIsImporting(false);
		form.resetForm();
		onClose();
	};

	const handleImportFile = (file: File) => {
		if (isImporting) {
			return;
		}
		setImportError(undefined);
		setImportFile(file);

		const format = secretsFileFormatFromFilename(file.name);
		if (!format) {
			setImportError({
				message:
					"Unsupported file type. Import a .env, .json, .yaml, or .yml file.",
			});
			return;
		}
		if (file.size > MaxSecretsFileBytes) {
			setImportError({
				message: "File is too large. Import a file of 1 MiB or smaller.",
			});
			return;
		}

		setIsImporting(true);
		const reader = new FileReader();
		reader.onload = async () => {
			const content = typeof reader.result === "string" ? reader.result : "";
			try {
				await onImportSecrets({ format, content });
				closeDialog();
			} catch (error) {
				setImportError(error);
			} finally {
				setIsImporting(false);
			}
		};
		reader.onerror = () => {
			setImportError({ message: "Failed to read the selected file." });
			setIsImporting(false);
		};
		reader.readAsText(file);
	};

	const request = secret
		? buildUpdateUserSecretRequest(secret, form.values, {
				clearValue: clearValueRequested,
				filePathEnabled,
			})
		: undefined;
	const hasUpdate = request ? Object.keys(request).length > 0 : false;
	const isBusy = isSubmitting || form.isSubmitting || isImporting;
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
								filePathEnabled={filePathEnabled}
								disableName
								showValue={false}
							/>
							{!filePathEnabled && secret.file_path !== "" && (
								<BlockedFilePathField
									storedFilePath={secret.file_path}
									isRemoved={form.values.file_path === ""}
									onRemove={() => {
										void form.setFieldValue("file_path", "", false);
									}}
									onRestore={() => {
										void form.setFieldValue(
											"file_path",
											secret.file_path,
											false,
										);
									}}
								/>
							)}
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
							<div className="flex flex-col gap-3">
								<FileUpload
									isUploading={isImporting}
									file={importFile}
									onUpload={handleImportFile}
									onUnsupportedFile={handleImportFile}
									onRemove={() => {
										setImportFile(undefined);
										setImportError(undefined);
									}}
									removeLabel="Remove file"
									title="Import secrets from a file"
									description="Import a single or multiple secrets at once with a .env, .json, .yaml, or .yml file."
									extensions={["env", "json", "yaml", "yml"]}
								/>
								{importError !== undefined && (
									<ImportSecretsError error={importError} />
								)}
							</div>
							<div className="flex items-center">
								<Separator className="flex-1" />
								<span className="whitespace-nowrap px-3 text-xs text-content-secondary">
									or add individually
								</span>
								<Separator className="flex-1" />
							</div>
							<SecretFields
								getFieldHelpers={getFieldHelpers}
								filePathEnabled={filePathEnabled}
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
	filePathEnabled: boolean;
	disableName?: boolean;
	showRequiredLabels?: boolean;
	showValue: boolean;
};

const SecretFields: FC<SecretFieldsProps> = ({
	getFieldHelpers,
	filePathEnabled,
	disableName,
	showRequiredLabels,
	showValue,
}) => {
	const envNameRequired = Boolean(showRequiredLabels && !filePathEnabled);
	const envNameHelperText = envNameRequired
		? "Required. File path delivery is disabled, so the secret needs an environment variable target."
		: filePathEnabled
			? "Optional. Exposes the secret as an environment variable with this name in your workspace."
			: "Environment variable delivery remains available while file path delivery is disabled.";

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
					helperText: envNameHelperText,
				})}
				label={
					envNameRequired ? (
						<RequiredFieldLabel>Environment variable</RequiredFieldLabel>
					) : (
						"Environment variable"
					)
				}
				placeholder="SERVICE_TOKEN"
				autoComplete="off"
				className="placeholder:text-content-disabled"
				aria-required={envNameRequired}
				data-lpignore="true"
				data-1p-ignore="true"
				data-form-type="other"
			/>
			{filePathEnabled && (
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
			)}
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

type BlockedFilePathFieldProps = {
	storedFilePath: string;
	isRemoved: boolean;
	onRemove: () => void;
	onRestore: () => void;
};

const BlockedFilePathField: FC<BlockedFilePathFieldProps> = ({
	storedFilePath,
	isRemoved,
	onRemove,
	onRestore,
}) => {
	return (
		<div className="flex flex-col gap-2">
			<span className="text-sm font-medium text-content-primary">
				File path
			</span>
			<div className="flex flex-col gap-2 sm:flex-row sm:items-center">
				<span
					className={cn(
						"flex-1 font-mono text-sm text-content-secondary",
						isRemoved && "line-through",
					)}
				>
					{storedFilePath}
				</span>
				<Button
					type="button"
					variant="outline"
					size="sm"
					className="shrink-0"
					onClick={isRemoved ? onRestore : onRemove}
				>
					{isRemoved ? "Keep file path" : "Remove file path"}
				</Button>
			</div>
			<span className="text-xs text-content-secondary">
				{isRemoved
					? "File path will be removed when you update. If this enabled secret has no environment variable, the same update will disable it."
					: "Your deployment administrator disabled file path secrets. This path stays saved but is not written to your workspaces, and it takes effect again if file path secrets are enabled."}
			</span>
		</div>
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
				<Textarea
					id={field.id}
					name={field.name}
					value={value}
					placeholder={placeholder}
					autoComplete="off"
					rows={4}
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
						"placeholder:text-content-disabled sm:flex-1 font-mono",
						displayField.error && "border-border-destructive",
						maskTypedValue && "[-webkit-text-security:disc]",
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

type ImportSecretsErrorProps = {
	error: unknown;
};

const ImportSecretsError: FC<ImportSecretsErrorProps> = ({ error }) => {
	const validations = getImportSecretValidations(error);
	if (validations.length === 0) {
		return <ErrorAlert error={error} showDebugDetail={false} />;
	}

	return (
		<Alert severity="error" prominent>
			<AlertTitle>
				{getErrorMessage(error, "Failed to import secrets.")}
			</AlertTitle>
			<AlertDescription>
				<ul className="m-0 flex list-disc flex-col gap-1 pl-5">
					{validations.map((validation) => (
						<li key={validation.field}>
							<span className="font-semibold">{validation.field}</span>
							<span className="block">{validation.detail}</span>
						</li>
					))}
				</ul>
			</AlertDescription>
		</Alert>
	);
};

function getImportSecretValidations(error: unknown): FieldError[] {
	if (isApiError(error)) {
		return error.response.data.validations ?? [];
	}
	if (isApiErrorResponse(error)) {
		return error.validations ?? [];
	}
	return [];
}
