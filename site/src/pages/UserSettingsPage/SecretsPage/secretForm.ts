import * as Yup from "yup";
import {
	type ApiErrorResponse,
	isApiError,
	isApiErrorResponse,
	mapApiErrorToFieldErrors,
} from "#/api/errors";
import {
	type CreateUserSecretRequest,
	MaxSecretValueSize,
	type UpdateUserSecretRequest,
	type UserSecret,
} from "#/api/typesGenerated";

interface SecretFormValues {
	name: string;
	value: string;
	description: string;
	env_name: string;
	file_path: string;
}

type SecretFormField = keyof SecretFormValues;

type SecretFieldErrors = Partial<Record<SecretFormField, string>>;

interface SecretFormErrors {
	fieldErrors: SecretFieldErrors;
	formError?: string;
}

const routeUnsafeSecretNameRegex = /[/?#]/;

// Env name, file path, and value validation rules mirror codersdk/usersecretvalidation.go.
// Keep these rules in sync with the backend validators.
const maxFilePathLength = 4096;
const posixEnvNameRegex = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

const reservedEnvNames = new Set([
	"PATH",
	"HOME",
	"SHELL",
	"USER",
	"LOGNAME",
	"PWD",
	"OLDPWD",
	"LANG",
	"TERM",
	"IFS",
	"CDPATH",
	"ENV",
	"BASH_ENV",
	"TMPDIR",
	"TMP",
	"TEMP",
	"HOSTNAME",
	"SSH_AUTH_SOCK",
	"SSH_CLIENT",
	"SSH_CONNECTION",
	"SSH_TTY",
	"EDITOR",
	"VISUAL",
	"PAGER",
	"VSCODE_PROXY_URI",
	"CS_DISABLE_GETTING_STARTED_OVERRIDE",
	"XDG_RUNTIME_DIR",
	"XDG_CONFIG_HOME",
	"XDG_DATA_HOME",
	"XDG_CACHE_HOME",
	"XDG_STATE_HOME",
]);

const reservedEnvPrefixes = ["GIT_", "LC_", "LD_", "DYLD_"];

export const validateUserSecretName = (name: string): string | undefined => {
	if (name.trim() === "") {
		return "Name is required.";
	}

	if (name.trim() !== name) {
		return "Name must not have leading or trailing whitespace.";
	}

	if (routeUnsafeSecretNameRegex.test(name)) {
		return "Name must not contain /, ?, or #.";
	}

	return undefined;
};

export const validateUserSecretEnvName = (
	envName: string,
): string | undefined => {
	if (envName === "") {
		return undefined;
	}

	if (!posixEnvNameRegex.test(envName)) {
		return "Environment variable name must start with a letter or underscore, followed by letters, digits, or underscores.";
	}

	const upper = envName.toUpperCase();
	if (reservedEnvNames.has(upper)) {
		return `${upper} is a reserved environment variable name.`;
	}

	if (upper === "CODER" || upper.startsWith("CODER_")) {
		return "CODER and CODER_* environment variable names are reserved for internal use.";
	}

	for (const prefix of reservedEnvPrefixes) {
		if (upper.startsWith(prefix)) {
			return `Environment variables starting with ${prefix} are reserved.`;
		}
	}

	return undefined;
};

export const validateUserSecretFilePath = (
	filePath: string,
): string | undefined => {
	if (filePath === "") {
		return undefined;
	}

	if (!filePath.startsWith("~/") && !filePath.startsWith("/")) {
		return "File path must start with ~/ or /.";
	}

	if (filePath.includes("\0")) {
		return "File path must not contain null bytes.";
	}

	if (byteLength(filePath) > maxFilePathLength) {
		return `File path must not exceed ${maxFilePathLength} bytes.`;
	}

	return undefined;
};

export const validateUserSecretValue = (value: string): string | undefined => {
	if (value.includes("\0")) {
		return "Secret value must not contain null bytes.";
	}

	if (byteLength(value) > MaxSecretValueSize) {
		return `Secret value must not exceed ${MaxSecretValueSize} bytes.`;
	}

	return undefined;
};

export const createSecretValidationSchema = Yup.object({
	name: Yup.string()
		.test("valid-secret-name", function (value) {
			const error = validateUserSecretName(value ?? "");
			return error ? this.createError({ message: error }) : true;
		}),
	value: Yup.string()
		.required("Value is required.")
		.test("valid-secret-value", function (value) {
			const error = validateUserSecretValue(value ?? "");
			return error ? this.createError({ message: error }) : true;
		}),
	description: Yup.string().default(""),
	env_name: Yup.string()
		.default("")
		.test("valid-env-name", function (value) {
			const error = validateUserSecretEnvName(value ?? "");
			return error ? this.createError({ message: error }) : true;
		}),
	file_path: Yup.string()
		.default("")
		.test("valid-file-path", function (value) {
			const error = validateUserSecretFilePath(value ?? "");
			return error ? this.createError({ message: error }) : true;
		}),
});

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

export const buildUpdateUserSecretRequest = (
	secret: UserSecret,
	values: SecretFormValues,
): UpdateUserSecretRequest => {
	return {
		...(values.value !== "" ? { value: values.value } : {}),
		...(values.description !== secret.description
			? { description: values.description }
			: {}),
		...(values.env_name !== secret.env_name
			? { env_name: values.env_name }
			: {}),
		...(values.file_path !== secret.file_path
			? { file_path: values.file_path }
			: {}),
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

export const getDuplicateSecretFieldErrors = (
	secrets: readonly UserSecret[],
	values: Pick<SecretFormValues, "name" | "env_name" | "file_path">,
	currentSecretID?: string,
): SecretFieldErrors => {
	const candidates = secrets.filter((secret) => secret.id !== currentSecretID);
	const errors: SecretFieldErrors = {};

	if (
		values.name !== "" &&
		candidates.some((secret) => secret.name === values.name)
	) {
		errors.name = "Name already in use.";
	}
	if (
		values.env_name !== "" &&
		candidates.some((secret) => secret.env_name === values.env_name)
	) {
		errors.env_name = "Variable already in use. Edit existing variable.";
	}
	if (
		values.file_path !== "" &&
		candidates.some((secret) => secret.file_path === values.file_path)
	) {
		errors.file_path = "File path already in use.";
	}

	return errors;
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

function byteLength(value: string): number {
	return new TextEncoder().encode(value).length;
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
