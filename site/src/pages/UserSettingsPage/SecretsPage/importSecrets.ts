import { parse as parseYaml } from "yaml";
import type { CreateUserSecretRequest, UserSecret } from "#/api/typesGenerated";
import {
	buildCreateUserSecretRequest,
	validateUserSecretEnvName,
	validateUserSecretFilePath,
	validateUserSecretName,
	validateUserSecretValue,
} from "./secretForm";

type CreateImportedSecret = (
	request: CreateUserSecretRequest,
) => Promise<UserSecret>;

type ImportSecretResult =
	| {
			status: "success";
			name: string;
			secret: UserSecret;
	  }
	| {
			status: "failure";
			name: string;
			error: unknown;
	  };

export const parseSecretImport = (
	content: string,
	fileName: string,
): CreateUserSecretRequest[] => {
	const lowerFileName = fileName.toLowerCase();
	if (lowerFileName.endsWith(".env")) {
		return validateImportedSecretRequests(parseEnvFile(content));
	}
	if (lowerFileName.endsWith(".json")) {
		return validateImportedSecretRequests(parseJsonFile(content));
	}
	if (lowerFileName.endsWith(".yaml") || lowerFileName.endsWith(".yml")) {
		return validateImportedSecretRequests(parseYamlFile(content));
	}

	throw new Error(
		"Unsupported file type. Expected .env, .json, .yaml, or .yml.",
	);
};

export const importUserSecretsSequential = async (
	requests: readonly CreateUserSecretRequest[],
	createSecret: CreateImportedSecret,
): Promise<ImportSecretResult[]> => {
	const results: ImportSecretResult[] = [];

	for (const request of requests) {
		try {
			const secret = await createSecret(request);
			results.push({
				status: "success",
				name: request.name,
				secret,
			});
		} catch (error) {
			results.push({
				status: "failure",
				name: request.name,
				error,
			});
		}
	}

	return results;
};

function parseEnvFile(content: string): CreateUserSecretRequest[] {
	const requests: CreateUserSecretRequest[] = [];
	const lines = content.split(/\r?\n/);

	for (const [index, rawLine] of lines.entries()) {
		const line = rawLine.trim();
		if (line === "" || line.startsWith("#")) {
			continue;
		}

		const assignmentLine = rawLine.trimStart().replace(/^export\s+/, "");
		const separatorIndex = assignmentLine.indexOf("=");
		if (separatorIndex <= 0) {
			throw new Error(
				`Invalid .env entry on line ${index + 1}: expected KEY=VALUE format.`,
			);
		}

		const name = assignmentLine.slice(0, separatorIndex).trim();
		const value = parseEnvValue(assignmentLine.slice(separatorIndex + 1));

		requests.push({
			name,
			env_name: name,
			value,
		});
	}

	return requests;
}

function parseEnvValue(rawValue: string): string {
	const value = rawValue.trim();
	if (value.length < 2) {
		return rawValue;
	}

	const quote = value[0];
	const isQuoted =
		(quote === '"' || quote === "'") && value[value.length - 1] === quote;
	if (!isQuoted) {
		return rawValue;
	}

	const unquoted = value.slice(1, -1);
	if (quote !== '"') {
		return unquoted;
	}

	const escapeMap: Record<string, string> = {
		n: "\n",
		r: "\r",
		t: "\t",
		'"': '"',
		"\\": "\\",
	};
	return unquoted.replace(/\\([nrt"\\])/g, (_, escaped: string) => {
		return escapeMap[escaped] ?? escaped;
	});
}

function parseJsonFile(content: string): CreateUserSecretRequest[] {
	const parsed: unknown = JSON.parse(content);
	return parseStructuredSecretImport(parsed, "JSON");
}

function parseYamlFile(content: string): CreateUserSecretRequest[] {
	const parsed: unknown = parseYaml(content);
	return parseStructuredSecretImport(parsed, "YAML");
}

function parseStructuredSecretImport(
	parsed: unknown,
	formatName: "JSON" | "YAML",
): CreateUserSecretRequest[] {
	if (Array.isArray(parsed)) {
		return parsed.map((item, index) =>
			parseStructuredSecretRequest(item, index, formatName),
		);
	}

	if (isRecord(parsed)) {
		return Object.entries(parsed).map(([name, value]) => {
			if (typeof value !== "string") {
				throw new Error(formatSecretValueError(formatName, name, value));
			}

			return {
				name,
				env_name: name,
				value,
			};
		});
	}

	throw new Error(
		`${formatName} secret imports must be an object or an array.`,
	);
}

function parseStructuredSecretRequest(
	item: unknown,
	index: number,
	formatName: "JSON" | "YAML",
): CreateUserSecretRequest {
	if (!isRecord(item)) {
		throw new Error(`Invalid ${formatName} secret entry at index ${index}.`);
	}

	const { name, value } = item;
	if (typeof name !== "string") {
		throw new Error(`Invalid ${formatName} secret name at index ${index}.`);
	}
	if (typeof value !== "string") {
		throw new Error(formatSecretValueError(formatName, name, value, index));
	}

	return buildCreateUserSecretRequest({
		name,
		value,
		description: getOptionalString(item.description),
		env_name: getOptionalString(item.env_name),
		file_path: getOptionalString(item.file_path),
	});
}

function validateImportedSecretRequests(
	requests: CreateUserSecretRequest[],
): CreateUserSecretRequest[] {
	const errors = requests.flatMap((request) => {
		const requestErrors: string[] = [];
		const nameError = validateUserSecretName(request.name);
		if (nameError) {
			requestErrors.push(
				formatImportFieldError(request.name, "name", nameError),
			);
		}

		const envNameError = validateUserSecretEnvName(request.env_name ?? "");
		if (envNameError) {
			requestErrors.push(
				formatImportFieldError(request.name, "env var", envNameError),
			);
		}

		const filePathError = validateUserSecretFilePath(request.file_path ?? "");
		if (filePathError) {
			requestErrors.push(
				formatImportFieldError(request.name, "file path", filePathError),
			);
		}

		const valueError = validateUserSecretValue(request.value);
		if (valueError) {
			requestErrors.push(
				formatImportFieldError(request.name, "value", valueError),
			);
		}

		return requestErrors;
	});

	if (errors.length > 0) {
		throw new Error(`Invalid secret import. ${errors.join(" ")}`);
	}

	return requests;
}

function formatImportFieldError(
	name: string,
	field: string,
	message: string,
): string {
	return `${name || "Secret"} ${field}: ${message}`;
}

function formatSecretValueError(
	formatName: "JSON" | "YAML",
	name: string,
	value: unknown,
	index?: number,
): string {
	const location = index === undefined ? "" : ` at index ${index}`;
	return `${formatName} secret value for ${name}${location} must be a string, got ${valueType(value)}.`;
}

function valueType(value: unknown): string {
	if (value === null) {
		return "null";
	}
	if (Array.isArray(value)) {
		return "array";
	}
	return typeof value;
}

function getOptionalString(value: unknown): string {
	return typeof value === "string" ? value : "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return (
		typeof value === "object" &&
		value !== null &&
		!Array.isArray(value) &&
		Object.getPrototypeOf(value) === Object.prototype
	);
}
