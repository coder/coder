import type { UserSecret } from "#/api/typesGenerated";
import { MockImportedUserSecret, mockApiError } from "#/testHelpers/entities";
import {
	buildCreateUserSecretRequest,
	buildImportSuccessMessage,
	buildUpdateUserSecretRequest,
	getCreateSecretRequiredFieldErrors,
	mapSecretApiErrorToFormErrors,
	secretsFileFormatFromFilename,
} from "./secretForm";

const existingSecrets: UserSecret[] = [
	{
		id: "11111111-1111-1111-1111-111111111111",
		name: "service-token",
		description: "Service token",
		env_name: "SERVICE_TOKEN",
		file_path: "",
		enabled: true,
		created_at: "2026-05-04T00:00:00Z",
		updated_at: "2026-05-04T00:00:00Z",
	},
	{
		id: "22222222-2222-2222-2222-222222222222",
		name: "service-key",
		description: "",
		env_name: "SERVICE_API_KEY",
		file_path: "~/.config/service/key",
		enabled: true,
		created_at: "2026-05-04T00:00:00Z",
		updated_at: "2026-05-04T00:00:00Z",
	},
];

describe("buildImportSuccessMessage", () => {
	it("reports a single secret imported successfully", () => {
		expect(buildImportSuccessMessage([MockImportedUserSecret])).toBe(
			"Imported 1 secret successfully.",
		);
	});

	it("reports multiple secrets imported successfully", () => {
		expect(
			buildImportSuccessMessage([
				MockImportedUserSecret,
				{ ...MockImportedUserSecret, id: "second-secret" },
			]),
		).toBe("Imported 2 secrets successfully.");
	});

	it("reports one secret imported without an env name", () => {
		expect(
			buildImportSuccessMessage([
				MockImportedUserSecret,
				{
					...MockImportedUserSecret,
					id: "without-env-name",
					env_name: "",
				},
			]),
		).toBe(
			"Imported 2 secrets. " +
				"1 was imported without an environment variable name " +
				"because its key is not a valid environment variable name. Edit it to set one.",
		);
	});

	it("reports multiple secrets imported without env names", () => {
		expect(
			buildImportSuccessMessage([
				{ ...MockImportedUserSecret, env_name: "" },
				{
					...MockImportedUserSecret,
					id: "second-without-env-name",
					env_name: "",
				},
				MockImportedUserSecret,
			]),
		).toBe(
			"Imported 3 secrets. " +
				"2 were imported without an environment variable name " +
				"because their keys are not valid environment variable names. Edit them to set one.",
		);
	});
});

describe("getCreateSecretRequiredFieldErrors", () => {
	it("requires name and value on create", () => {
		expect(
			getCreateSecretRequiredFieldErrors({
				name: "",
				value: "",
			}),
		).toEqual({
			name: "Name is required.",
			value: "Value is required.",
		});
	});

	it("requires a non-whitespace name", () => {
		expect(
			getCreateSecretRequiredFieldErrors({
				name: "   ",
				value: "some value",
			}),
		).toEqual({
			name: "Name is required.",
		});
	});
});

describe("payload builders", () => {
	it("builds create payloads from form values", () => {
		expect(
			buildCreateUserSecretRequest({
				name: "service-token",
				value: "example-value",
				description: "Service token",
				env_name: "SERVICE_TOKEN",
				file_path: "",
			}),
		).toEqual({
			name: "service-token",
			value: "example-value",
			description: "Service token",
			env_name: "SERVICE_TOKEN",
		});
	});

	it("sends only changed update fields", () => {
		expect(
			buildUpdateUserSecretRequest(existingSecrets[0], {
				name: "service-token",
				value: "",
				description: "Updated description",
				env_name: "SERVICE_TOKEN",
				file_path: "~/secrets/service-token",
			}),
		).toEqual({
			description: "Updated description",
			file_path: "~/secrets/service-token",
		});
	});

	it("includes replacement values only when provided", () => {
		expect(
			buildUpdateUserSecretRequest(existingSecrets[0], {
				name: "service-token",
				value: "replacement-value",
				description: "Service token",
				env_name: "SERVICE_TOKEN",
				file_path: "",
			}),
		).toEqual({
			value: "replacement-value",
		});
	});

	it("sends an empty value when clearing an update", () => {
		expect(
			buildUpdateUserSecretRequest(
				existingSecrets[0],
				{
					name: "service-token",
					value: "",
					description: "Service token",
					env_name: "SERVICE_TOKEN",
					file_path: "",
				},
				{ clearValue: true },
			),
		).toEqual({
			value: "",
		});
	});
});

describe("secretsFileFormatFromFilename", () => {
	it.each([
		["a.env", "env"],
		[".env", "env"],
		["prod.env", "env"],
		["config.json", "json"],
		["values.yaml", "yaml"],
		["values.yml", "yaml"],
		["CONFIG.JSON", "json"],
		["Values.YML", "yaml"],
		["secrets.ENV", "env"],
	])("maps %s to the %s format", (filename, format) => {
		expect(secretsFileFormatFromFilename(filename)).toBe(format);
	});

	it.each([["foo.txt"], ["noextension"], ["archive.tar.gz"], [""]])(
		"returns undefined for unsupported filename %s",
		(filename) => {
			expect(secretsFileFormatFromFilename(filename)).toBeUndefined();
		},
	);
});

describe("mapSecretApiErrorToFormErrors", () => {
	it("maps structured API validation errors to fields", () => {
		expect(
			mapSecretApiErrorToFormErrors(
				mockApiError({
					message: "Validation failed.",
					validations: [
						{ field: "name", detail: "Name already in use." },
						{ field: "env_name", detail: "Use a different variable." },
						{ field: "file_path", detail: "Use an absolute path." },
						{ field: "unknown", detail: "Ignored." },
					],
				}),
			).fieldErrors,
		).toEqual({
			name: "Name already in use.",
			env_name: "Use a different variable.",
			file_path: "Use an absolute path.",
		});
	});

	it("maps unstructured API validation errors to a form error", () => {
		expect(
			mapSecretApiErrorToFormErrors(
				mockApiError({
					message: "Invalid environment variable name.",
					detail: "Backend detail.",
				}),
			),
		).toEqual({
			fieldErrors: {},
			formError: "Backend detail.",
		});
	});

	it("maps generic create conflicts to a form error", () => {
		expect(
			mapSecretApiErrorToFormErrors({
				isAxiosError: true,
				status: 409,
				response: {
					status: 409,
					data: {
						message:
							"A secret with that name, environment variable, or file path already exists.",
					},
				},
			}).formError,
		).toBe(
			"A secret with that name, environment variable, or file path already exists.",
		);
	});
});
