import { mockApiError } from "testHelpers/entities";
import {
	getErrorDetail,
	getErrorMessage,
	getValidationErrorMessage,
	isApiError,
	mapApiErrorToFieldErrors,
} from "./errors";

describe("isApiError", () => {
	it("returns true when the object is an API Error", () => {
		expect(
			isApiError(
				mockApiError({
					message: "Invalid entry",
					validations: [
						{ detail: "Username is already in use", field: "username" },
					],
				}),
			),
		).toBe(true);
	});

	it("returns false when the object is Error", () => {
		expect(isApiError(new Error())).toBe(false);
	});

	it("returns false when the object is undefined", () => {
		expect(isApiError(undefined)).toBe(false);
	});
});

describe("mapApiErrorToFieldErrors", () => {
	it("returns correct field errors", () => {
		expect(
			mapApiErrorToFieldErrors({
				message: "Invalid entry",
				validations: [
					{ detail: "Username is already in use", field: "username" },
				],
			}),
		).toEqual({
			username: "Username is already in use",
		});
	});
});

describe("getValidationErrorMessage", () => {
	it("returns multiple validation messages", () => {
		expect(
			getValidationErrorMessage(
				mockApiError({
					message: "Invalid user search query.",
					validations: [
						{
							field: "status",
							detail: `Query param "status" has invalid value: "inactive" is not a valid user status`,
						},
						{
							field: "q",
							detail: `Query element "role:a:e" can only contain 1 ':'`,
						},
					],
				}),
			),
		).toEqual(
			`Query param "status" has invalid value: "inactive" is not a valid user status\nQuery element "role:a:e" can only contain 1 ':'`,
		);
	});

	it("non-API error returns empty validation message", () => {
		expect(
			getValidationErrorMessage(new Error("Invalid user search query.")),
		).toEqual("");
	});

	it("no validations field returns empty validation message", () => {
		expect(
			getValidationErrorMessage(
				mockApiError({
					message: "Invalid user search query.",
					detail: `Query element "role:a:e" can only contain 1 ':'`,
				}),
			),
		).toEqual("");
	});

	it("returns default message for error that is empty string", () => {
		expect(getErrorMessage("", "Something went wrong.")).toBe(
			"Something went wrong.",
		);
	});

	it("returns default message for 404 API response", () => {
		expect(
			getErrorMessage(
				mockApiError({
					message: "",
				}),
				"Something went wrong.",
			),
		).toBe("Something went wrong.");
	});
});

describe("getErrorDetail", () => {
	it("returns detail field when present", () => {
		expect(
			getErrorDetail(
				mockApiError({
					message: "Error message",
					detail: "Detailed explanation",
				}),
			),
		).toBe("Detailed explanation");
	});

	it("returns validation messages when no detail but validations present", () => {
		expect(
			getErrorDetail(
				mockApiError({
					message: "Unable to validate presets",
					validations: [
						{
							field: "Custom Image (Missing URL)",
							detail:
								"Parameter custom_image_url: Required parameter not provided",
						},
					],
				}),
			),
		).toBe(
			"Custom Image (Missing URL): Parameter custom_image_url: Required parameter not provided",
		);
	});

	it("returns multiple validation messages joined by newlines", () => {
		expect(
			getErrorDetail(
				mockApiError({
					message: "Unable to validate presets",
					validations: [
						{ field: "preset1", detail: "Missing required parameter" },
						{ field: "preset2", detail: "Invalid value" },
					],
				}),
			),
		).toBe("preset1: Missing required parameter\npreset2: Invalid value");
	});

	it("returns generic message for Error without validations", () => {
		expect(getErrorDetail(new Error("Something failed"))).toBe(
			"Please check the developer console for more details.",
		);
	});

	it("returns undefined for non-error values", () => {
		expect(getErrorDetail(null)).toBeUndefined();
		expect(getErrorDetail(undefined)).toBeUndefined();
		expect(getErrorDetail("string")).toBeUndefined();
	});
});
