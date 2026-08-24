import {
	type ApiErrorResponse,
	isApiError,
	isApiErrorResponse,
	mapApiErrorToFieldErrors,
} from "#/api/errors";
import type {
	CreateUserSecretRequest,
	SecretsFileFormat,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";

export interface SecretFormValues {
	name: string;
	value: string;
	description: string;
	env_name: string;
	file_path: string;
}

type SecretFormField = keyof SecretFormValues;

export type SecretFieldErrors = Partial<Record<SecretFormField, string>>;

interface SecretFormErrors {
	fieldErrors: SecretFieldErrors;
	formError?: string;
}

export const buildImportSuccessMessage = (secrets: UserSecret[]): string => {
	const total = secrets.length;
	const noEnvName = secrets.filter((s) => s.env_name === "").length;
	const secretWord = total === 1 ? "secret" : "secrets";
	if (noEnvName === 0) {
		return `Imported ${total} ${secretWord} successfully.`;
	}
	const wasWere = noEnvName === 1 ? "was" : "were";
	const keyPhrase =
		noEnvName === 1
			? "its key is not a valid environment variable name. Edit it to set one."
			: "their keys are not valid environment variable names. Edit them to set one.";
	return (
		`Imported ${total} ${secretWord}. ` +
		`${noEnvName} ${wasWere} imported without an environment variable name ` +
		`because ${keyPhrase}`
	);
};

export const secretsFileFormatFromFilename = (
	filename: string,
): SecretsFileFormat | undefined => {
	const lowerName = filename.toLowerCase();
	if (lowerName.endsWith(".env")) {
		return "env";
	}
	if (lowerName.endsWith(".json")) {
		return "json";
	}
	if (lowerName.endsWith(".yaml") || lowerName.endsWith(".yml")) {
		return "yaml";
	}
	return undefined;
};

export const getCreateSecretRequiredFieldErrors = (
	values: Pick<SecretFormValues, "name" | "value" | "env_name">,
	filePathEnabled = true,
): SecretFieldErrors => {
	const errors: SecretFieldErrors = {};
	if (values.name.trim() === "") {
		errors.name = "Name is required.";
	}
	if (!filePathEnabled && values.env_name.trim() === "") {
		errors.env_name =
			"Environment variable is required when file path delivery is disabled.";
	}
	if (values.value === "") {
		errors.value = "Value is required.";
	}
	return errors;
};

type SecretTypeLabel = "env var" | "file" | "env var + file" | "not injected";

export interface SecretInjectionSummary {
	injectsEnv: boolean;
	injectsFile: boolean;
	hasBlockedFilePath: boolean;
	canEnable: boolean;
	typeLabel: SecretTypeLabel;
}

// A deployment can disable file-path secrets. Stored file paths survive that
// setting and become effective again once an administrator re-enables it, so
// the effective targets are derived instead of read straight off the secret.
export const getSecretInjectionSummary = (
	secret: Pick<UserSecret, "env_name" | "file_path">,
	filePathEnabled: boolean,
): SecretInjectionSummary => {
	const injectsEnv = secret.env_name !== "";
	const hasFilePath = secret.file_path !== "";
	const injectsFile = hasFilePath && filePathEnabled;

	return {
		injectsEnv,
		injectsFile,
		hasBlockedFilePath: hasFilePath && !filePathEnabled,
		canEnable: injectsEnv || injectsFile,
		typeLabel: getSecretTypeLabel(injectsEnv, injectsFile),
	};
};

function getSecretTypeLabel(
	injectsEnv: boolean,
	injectsFile: boolean,
): SecretTypeLabel {
	if (injectsEnv && injectsFile) {
		return "env var + file";
	}
	if (injectsEnv) {
		return "env var";
	}
	if (injectsFile) {
		return "file";
	}
	return "not injected";
}

export const buildCreateUserSecretRequest = (
	values: SecretFormValues,
): CreateUserSecretRequest => {
	return stripEmptyOptionalFields({
		name: values.name,
		value: values.value,
		description: values.description,
		env_name: values.env_name,
		file_path: values.file_path,
	});
};

type BuildUpdateUserSecretRequestOptions = {
	clearValue?: boolean;
	filePathEnabled?: boolean;
};

export const buildUpdateUserSecretRequest = (
	secret: UserSecret,
	values: SecretFormValues,
	options: BuildUpdateUserSecretRequestOptions = {},
): UpdateUserSecretRequest => {
	const removesBlockedOnlyTarget =
		options.filePathEnabled === false &&
		secret.enabled &&
		secret.file_path !== "" &&
		values.file_path === "" &&
		values.file_path !== secret.file_path &&
		values.env_name === "";

	return {
		...(options.clearValue
			? { value: "" }
			: values.value !== ""
				? { value: values.value }
				: {}),
		...(values.description !== secret.description
			? { description: values.description }
			: {}),
		...(values.env_name !== secret.env_name
			? { env_name: values.env_name }
			: {}),
		...(values.file_path !== secret.file_path
			? { file_path: values.file_path }
			: {}),
		...(removesBlockedOnlyTarget ? { enabled: false } : {}),
	};
};

export const mapSecretApiErrorToFormErrors = (
	error: unknown,
): SecretFormErrors => {
	const apiError = getApiError(error);
	if (!apiError) {
		return {
			fieldErrors: {},
			formError: "Something went wrong.",
		};
	}

	const fieldErrors = getSecretFieldErrors(apiError.response);
	if (Object.keys(fieldErrors).length > 0) {
		return { fieldErrors };
	}

	return {
		fieldErrors: {},
		formError: apiError.response.detail ?? apiError.response.message,
	};
};

const secretFormFieldLookup: Record<SecretFormField, true> = {
	name: true,
	value: true,
	description: true,
	env_name: true,
	file_path: true,
};

function getSecretFieldErrors(response: ApiErrorResponse): SecretFieldErrors {
	const apiFieldErrors = mapApiErrorToFieldErrors(response);
	const fieldErrors: SecretFieldErrors = {};
	for (const [field, message] of Object.entries(apiFieldErrors)) {
		if (isSecretFormField(field)) {
			fieldErrors[field] = message;
		}
	}
	return fieldErrors;
}

function isSecretFormField(field: string): field is SecretFormField {
	return Object.hasOwn(secretFormFieldLookup, field);
}

function getApiError(
	error: unknown,
): { status?: number; response: ApiErrorResponse } | undefined {
	if (isApiError(error)) {
		return {
			status: error.response.status ?? error.status,
			response: error.response.data,
		};
	}

	if (isApiErrorResponse(error)) {
		return {
			response: error,
		};
	}

	return undefined;
}

function stripEmptyOptionalFields(
	request: CreateUserSecretRequest,
): CreateUserSecretRequest {
	return {
		name: request.name,
		value: request.value,
		...(request.description ? { description: request.description } : {}),
		...(request.env_name ? { env_name: request.env_name } : {}),
		...(request.file_path ? { file_path: request.file_path } : {}),
	};
}
