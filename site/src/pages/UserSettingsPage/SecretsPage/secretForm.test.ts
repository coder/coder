import type { UserSecret } from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
import {
	buildCreateUserSecretRequest,
	buildUpdateUserSecretRequest,
	getCreateSecretRequiredFieldErrors,
	mapSecretApiErrorToFormErrors,
} from "./secretForm";

const existingSecrets: UserSecret[] = [
	{
		id: "11111111-1111-1111-1111-111111111111",
		name: "service-token",
		description: "Service token",
		env_name: "SERVICE_TOKEN",
		file_path: "",
		created_at: "2026-05-04T00:00:00Z",
		updated_at: "2026-05-04T00:00:00Z",
	},
	{
		id: "22222222-2222-2222-2222-222222222222",
		name: "service-key",
		description: "",
		env_name: "SERVICE_API_KEY",
		file_path: "~/.config/service/key",
		created_at: "2026-05-04T00:00:00Z",
		updated_at: "2026-05-04T00:00:00Z",
	},
];

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
