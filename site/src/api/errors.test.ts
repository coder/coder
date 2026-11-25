import { mockApiError } from "#/testHelpers/entities";
import {
	getErrorMessage,
	getValidationErrorMessage,
	isApiError,
	isWorkspaceNotFound,
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

describe("isWorkspaceNotFound", () => {
	it("returns true for Axios-style 404 Not Found errors", () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 404,
				data: { message: "Workspace not found" },
			},
		};

		expect(isWorkspaceNotFound(error)).toBe(true);
	});

	it("returns true for Axios-style 410 errors", () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 410,
				data: { message: "Workspace gone" },
			},
		};

		expect(isWorkspaceNotFound(error)).toBe(true);
	});

	it("returns false for Axios-style non-404-or-410 errors", () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 500,
				data: { message: "Internal server error" },
			},
		};

		expect(isWorkspaceNotFound(error)).toBe(false);
	});

	it("returns false for axios errors without a response (network error)", () => {
		const error = {
			isAxiosError: true,
			response: undefined,
		};

		expect(isWorkspaceNotFound(error)).toBe(false);
	});

	it("returns false for plain Error objects", () => {
		expect(isWorkspaceNotFound(new Error("Workspace not found"))).toBe(false);
	});

	it("returns false for non-error values", () => {
		expect(isWorkspaceNotFound("nope")).toBe(false);
		expect(isWorkspaceNotFound(null)).toBe(false);
	});
});
