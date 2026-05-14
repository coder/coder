import { parse as parseYaml } from "yaml";
import type { CreateUserSecretRequest, UserSecret } from "#/api/typesGenerated";
import {
	buildCreateUserSecretRequest,
	validateUserSecretName,
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
		return parseEnvFile(content);
	}
	if (lowerFileName.endsWith(".json")) {
		return parseJsonFile(content);
	}
	if (lowerFileName.endsWith(".yaml") || lowerFileName.endsWith(".yml")) {
		return parseYamlFile(content);
	}

	throw new Error("Unsupported secret import file type.");
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

		const separatorIndex = line.indexOf("=");
		if (separatorIndex <= 0) {
			throw new Error(`Invalid .env entry on line ${index + 1}.`);
		}

		const name = line.slice(0, separatorIndex).trim();
		const nameError = validateUserSecretName(name);
		if (nameError) {
			throw new Error(`Invalid .env entry on line ${index + 1}.`);
		}
		const value = line.slice(separatorIndex + 1);

		requests.push({
			name,
			env_name: name,
			value,
		});
	}

	return requests;
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
			const nameError = validateUserSecretName(name);
			if (nameError) {
				throw new Error(`Invalid ${formatName} secret name for ${name}.`);
			}
			if (typeof value !== "string") {
				throw new Error(`Invalid ${formatName} secret value for ${name}.`);
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
	if (typeof name !== "string" || validateUserSecretName(name)) {
		throw new Error(`Invalid ${formatName} secret name at index ${index}.`);
	}
	if (typeof value !== "string") {
		throw new Error(`Invalid ${formatName} secret value for ${name}.`);
	}

	return buildCreateUserSecretRequest({
		name,
		value,
		description: getOptionalString(item.description),
		env_name: getOptionalString(item.env_name),
		file_path: getOptionalString(item.file_path),
	});
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
