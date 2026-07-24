import {
	type ApiErrorResponse,
	isApiError,
	isApiErrorResponse,
	mapApiErrorToFieldErrors,
} from "#/api/errors";
import type {
	CreateUserSecretRequest,
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

export const getCreateSecretRequiredFieldErrors = (
	values: Pick<SecretFormValues, "name" | "value">,
): SecretFieldErrors => {
	const errors: SecretFieldErrors = {};
	if (values.name.trim() === "") {
		errors.name = "Name is required.";
	}
	if (values.value === "") {
		errors.value = "Value is required.";
	}
	return errors;
};

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
};

export const buildUpdateUserSecretRequest = (
	secret: UserSecret,
	values: SecretFormValues,
	options: BuildUpdateUserSecretRequestOptions = {},
): UpdateUserSecretRequest => {
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
