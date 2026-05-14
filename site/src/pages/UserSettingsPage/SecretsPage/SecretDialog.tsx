import { type FormikTouched, useFormik } from "formik";
import { UploadIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import { toast } from "sonner";
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
	importUserSecretsSequential,
	parseSecretImport,
} from "./importSecrets";
import {
	buildCreateUserSecretRequest,
	buildUpdateUserSecretRequest,
	createSecretValidationSchema,
	getDuplicateSecretFieldErrors,
	mapSecretApiErrorToFormErrors,
	type SecretFieldErrors,
	type SecretFormValues,
	updateSecretValidationSchema,
} from "./secretForm";

export type CreateSecretOptions = {
	showToast?: boolean;
};

type SecretDialogProps = {
	open: boolean;
	secret?: UserSecret;
	secrets: readonly UserSecret[];
	isSubmitting: boolean;
	onClose: () => void;
	onCreateSecret: (
		request: CreateUserSecretRequest,
		options?: CreateSecretOptions,
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
	"Secrets are encrypted and cannot be retrieved after creation. Make sure to save the secret value in a secure location.";

const importSecretFileTypes = ".env,.json,.yaml,.yml";

export const SecretDialog: FC<SecretDialogProps> = ({
	open,
	secret,
	secrets,
	isSubmitting,
	onClose,
	onCreateSecret,
	onUpdateSecret,
}) => {
	const isEdit = Boolean(secret);
	const [isImporting, setIsImporting] = useState(false);
	const [replacementFileName, setReplacementFileName] = useState("");
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
		validationSchema: isEdit
			? updateSecretValidationSchema
			: createSecretValidationSchema,
		validate: (values) =>
			getDuplicateSecretFieldErrors(secrets, values, secret?.id),
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
				setReplacementFileName("");
				onClose();
			} catch (error) {
				const formErrors = mapSecretApiErrorToFormErrors(error);
				helpers.setErrors(formErrors.fieldErrors);
				helpers.setTouched(touchedFromFieldErrors(formErrors.fieldErrors));
				helpers.setStatus(formErrors.formError);
			}
		},
	});

	const request = secret
		? buildUpdateUserSecretRequest(secret, form.values)
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
				if (!nextOpen) {
					if (isImporting) {
						return;
					}

					form.resetForm();
					setReplacementFileName("");
					onClose();
				}
			}}
		>
			<DialogContent
				className="max-h-[90vh] overflow-y-auto"
				aria-describedby={undefined}
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
							<UploadBlock
								description="Or, replace your current value with a new file."
								buttonLabel="Upload value"
								onFile={async (file) => {
									try {
										setReplacementFileName(file.name);
										await form.setFieldValue("value", await file.text());
									} catch (error) {
										form.setStatus(messageFromUnknown(error));
									}
								}}
							/>
							{replacementFileName && (
								<p className="m-0 text-xs text-content-secondary">
									Replacement value selected from {replacementFileName}.
								</p>
							)}
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
							<UploadBlock
								description="Import a single or multiple secrets at once with a .env, .json, .yaml, or .yml file."
								accept={importSecretFileTypes}
								disabled={isImporting}
								loading={isImporting}
								onFile={async (file) => {
									form.setStatus(undefined);
									setIsImporting(true);
									try {
										const requests = parseSecretImport(
											await file.text(),
											file.name,
										);
										if (requests.length === 0) {
											form.setStatus("No secrets found to import.");
											return;
										}

										const results = await importUserSecretsSequential(
											requests,
											async (request) =>
												(await onCreateSecret(request, {
													showToast: false,
												})) as UserSecret,
										);
										const failures = results.filter(
											(result) => result.status === "failure",
										);
										if (failures.length > 0) {
											const successCount = results.length - failures.length;
											const message =
												successCount > 0
													? `Imported ${successCount} of ${results.length} secrets.`
													: "No secrets imported.";
											const description = `Failed to import ${formatSecretNameList(
												failures.map((failure) => failure.name),
											)}.`;
											toast.error(message, { description });
											form.setStatus(`${message} ${description}`);
											return;
										}

										toast.success(
											`Imported ${formatSecretCount(results.length)} successfully.`,
										);
										form.resetForm();
										onClose();
									} catch (error) {
										form.setStatus(messageFromUnknown(error));
									} finally {
										setIsImporting(false);
									}
								}}
							/>
							<div className="flex items-center gap-3 text-xs uppercase text-content-secondary">
								<div className="h-px flex-1 bg-border" />
								or add individually
								<div className="h-px flex-1 bg-border" />
							</div>
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
								setReplacementFileName("");
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

type UploadBlockProps = {
	description: string;
	accept?: string;
	buttonLabel?: string;
	disabled?: boolean;
	loading?: boolean;
	onFile: (file: File) => Promise<void> | void;
};

const UploadBlock: FC<UploadBlockProps> = ({
	description,
	accept,
	buttonLabel = "Upload",
	disabled,
	loading,
	onFile,
}) => {
	const inputRef = useRef<HTMLInputElement>(null);

	return (
		<div className="rounded-md border border-dashed border-border p-4">
			<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="flex flex-col gap-1">
					<p className="m-0 text-sm font-medium text-content-primary">
						{description}
					</p>
				</div>
				<Button
					type="button"
					variant="outline"
					disabled={disabled || loading}
					aria-busy={loading || undefined}
					onClick={() => inputRef.current?.click()}
				>
					<Spinner loading={loading}>
						<UploadIcon />
					</Spinner>
					{buttonLabel}
				</Button>
			</div>
			<input
				ref={inputRef}
				className="hidden"
				data-testid="secret-upload-input"
				type="file"
				tabIndex={-1}
				aria-hidden="true"
				accept={accept}
				disabled={disabled || loading}
				onChange={async (event) => {
					const file = event.currentTarget.files?.[0];
					event.currentTarget.value = "";
					if (file && !disabled && !loading) {
						await onFile(file);
					}
				}}
			/>
		</div>
	);
};

function formatSecretCount(count: number): string {
	return `${count} ${count === 1 ? "secret" : "secrets"}`;
}

function formatSecretNameList(names: readonly string[]): string {
	const maxNames = 3;
	const formattedNames = names.slice(0, maxNames).map(formatSecretName);
	const remainingNames = names.length - formattedNames.length;
	if (remainingNames > 0) {
		return `${formattedNames.join(", ")}, and ${remainingNames} more`;
	}
	return formattedNames.join(", ");
}

function formatSecretName(name: string): string {
	const maxLength = 80;
	if (name.length <= maxLength) {
		return name;
	}
	return `${name.slice(0, maxLength - 3)}...`;
}

function touchedFromFieldErrors(
	fieldErrors: SecretFieldErrors,
): FormikTouched<SecretFormValues> {
	return Object.fromEntries(
		Object.keys(fieldErrors).map((field) => [field, true]),
	) as FormikTouched<SecretFormValues>;
}

function messageFromUnknown(error: unknown): string {
	return error instanceof Error ? error.message : "Unable to import secrets.";
}
