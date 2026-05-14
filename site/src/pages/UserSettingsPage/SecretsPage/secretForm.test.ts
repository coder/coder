import type { UserSecret } from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
import {
	buildCreateUserSecretRequest,
	buildUpdateUserSecretRequest,
	createSecretValidationSchema,
	getDuplicateSecretFieldErrors,
	mapSecretApiErrorToFormErrors,
	validateUserSecretEnvName,
	validateUserSecretFilePath,
	validateUserSecretName,
} from "./secretForm";

const existingSecrets: UserSecret[] = [
	{
		id: "11111111-1111-1111-1111-111111111111",
		name: "github",
		description: "GitHub token",
		env_name: "GITHUB_TOKEN",
		file_path: "",
		created_at: "2026-05-04T00:00:00Z",
		updated_at: "2026-05-04T00:00:00Z",
	},
	{
		id: "22222222-2222-2222-2222-222222222222",
		name: "anthropic",
		description: "",
		env_name: "ANTHROPIC_API_KEY",
		file_path: "~/.config/anthropic/key",
		created_at: "2026-05-04T00:00:00Z",
		updated_at: "2026-05-04T00:00:00Z",
	},
];

describe("createSecretValidationSchema", () => {
	it("requires name and value on create", async () => {
		await expect(
			createSecretValidationSchema.validate(
				{
					name: "",
					value: "",
					description: "",
					env_name: "",
					file_path: "",
				},
				{ abortEarly: false },
			),
		).rejects.toMatchObject({
			inner: expect.arrayContaining([
				expect.objectContaining({ path: "name" }),
				expect.objectContaining({ path: "value" }),
			]),
		});
	});

	it.each([
		"foo/bar",
		"foo?bar",
		"foo#bar",
	])("rejects route-unsafe secret name %s", (name) => {
		expect(validateUserSecretName(name)).toContain("must not contain");
	});

	it("rejects whitespace-only secret names", () => {
		expect(validateUserSecretName("   ")).toBe("Name is required.");
	});

	it.each([
		" github",
		"github ",
	])("rejects leading/trailing whitespace secret name %j", (name) => {
		expect(validateUserSecretName(name)).toBe(
			"Name must not have leading or trailing whitespace.",
		);
	});

	it.each([
		"GITHUB_TOKEN",
		"ANTHROPIC_API_KEY",
		"_EXAMPLE_TOKEN",
	])("allows valid uppercase env var %s", (envName) => {
		expect(validateUserSecretEnvName(envName)).toBeUndefined();
	});

	it("rejects env vars that start with a digit", () => {
		expect(validateUserSecretEnvName("1EXAMPLE_TOKEN")).toContain(
			"must start with",
		);
	});

	it("rejects reserved env vars", () => {
		expect(validateUserSecretEnvName("PATH")).toContain("reserved");
	});

	it("rejects the CODER namespace", () => {
		expect(validateUserSecretEnvName("CODER_WORKSPACE_NAME")).toContain(
			"CODER_",
		);
	});

	it("allows empty, absolute, and home-relative file paths", () => {
		expect(validateUserSecretFilePath("")).toBeUndefined();
		expect(validateUserSecretFilePath("/usr/local/secrets")).toBeUndefined();
		expect(validateUserSecretFilePath("~/secrets/example")).toBeUndefined();
	});

	it("rejects relative file paths", () => {
		expect(validateUserSecretFilePath("secrets/example")).toContain(
			"must start",
		);
	});
});

describe("payload builders", () => {
	it("builds create payloads from form values", () => {
		expect(
			buildCreateUserSecretRequest({
				name: "github",
				value: "example-value",
				description: "GitHub token",
				env_name: "GITHUB_TOKEN",
				file_path: "",
			}),
		).toEqual({
			name: "github",
			value: "example-value",
			description: "GitHub token",
			env_name: "GITHUB_TOKEN",
		});
	});

	it("sends only changed update fields", () => {
		expect(
			buildUpdateUserSecretRequest(existingSecrets[0], {
				name: "github",
				value: "",
				description: "Updated description",
				env_name: "GITHUB_TOKEN",
				file_path: "~/secrets/github",
			}),
		).toEqual({
			description: "Updated description",
			file_path: "~/secrets/github",
		});
	});

	it("includes replacement values only when provided", () => {
		expect(
			buildUpdateUserSecretRequest(existingSecrets[0], {
				name: "github",
				value: "replacement-value",
				description: "GitHub token",
				env_name: "GITHUB_TOKEN",
				file_path: "",
			}),
		).toEqual({
			value: "replacement-value",
		});
	});
});

describe("mapSecretApiErrorToFormErrors", () => {
	it.each([
		["Name is required.", "name"],
		["Value is required.", "value"],
		["Invalid secret value.", "value"],
		["Invalid environment variable name.", "env_name"],
		["Invalid file path.", "file_path"],
	])("maps backend 400 message %s", (message, field) => {
		expect(
			mapSecretApiErrorToFormErrors(
				mockApiError({
					message,
					detail: "Backend detail.",
				}),
			).fieldErrors,
		).toEqual({
			[field]: "Backend detail.",
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

describe("getDuplicateSecretFieldErrors", () => {
	it("maps duplicate names to the name field", () => {
		expect(
			getDuplicateSecretFieldErrors(existingSecrets, {
				name: "github",
				env_name: "",
				file_path: "",
			}),
		).toEqual({
			name: "Name already in use.",
		});
	});

	it("maps duplicate env vars to the env var field", () => {
		expect(
			getDuplicateSecretFieldErrors(existingSecrets, {
				name: "new-secret",
				env_name: "GITHUB_TOKEN",
				file_path: "",
			}),
		).toEqual({
			env_name: "Variable already in use. Edit existing variable.",
		});
	});

	it("maps duplicate file paths to the file path field", () => {
		expect(
			getDuplicateSecretFieldErrors(existingSecrets, {
				name: "new-secret",
				env_name: "",
				file_path: "~/.config/anthropic/key",
			}),
		).toEqual({
			file_path: "File path already in use.",
		});
	});

	it("ignores the current secret by id when editing", () => {
		expect(
			getDuplicateSecretFieldErrors(
				existingSecrets,
				{
					name: "github",
					env_name: "GITHUB_TOKEN",
					file_path: "",
				},
				existingSecrets[0].id,
			),
		).toEqual({});
	});
});
