import { describe, expect, test } from "bun:test";
import { ActionInputs, ActionInputsSchema } from "./schemas";

const actionInputValid: ActionInputs = {
	coderURL: "https://coder.test",
	coderToken: "test-token",
	coderOrganization: "my-org",
	coderTaskNamePrefix: "gh",
	coderTaskPrompt: "test prompt",
	coderTemplateName: "test-template",
	githubIssueURL: "https://github.com/owner/repo/issues/123",
	githubToken: "github-token",
	githubUserID: 12345,
	coderTemplatePreset: "",
};

describe("ActionInputsSchema", () => {
	describe("Valid Input Cases", () => {
		test("accepts minimal required inputs and sets default values correctly", () => {
			const result = ActionInputsSchema.parse(actionInputValid);
			expect(result.coderURL).toBe(actionInputValid.coderURL);
			expect(result.coderToken).toBe(actionInputValid.coderToken);
			expect(result.coderOrganization).toBe(actionInputValid.coderOrganization);
			expect(result.coderTaskNamePrefix).toBe(
				actionInputValid.coderTaskNamePrefix,
			);
			expect(result.coderTaskPrompt).toBe(actionInputValid.coderTaskPrompt);
			expect(result.coderTemplateName).toBe(actionInputValid.coderTemplateName);
			expect(result.githubIssueURL).toBe(actionInputValid.githubIssueURL);
			expect(result.githubToken).toBe(actionInputValid.githubToken);
			expect(result.githubUserID).toBe(actionInputValid.githubUserID);
			expect(result.coderTemplatePreset).toBeEmpty();
		});

		test("accepts all optional inputs", () => {
			const input: ActionInputs = {
				...actionInputValid,
				coderTemplatePreset: "custom",
			};
			const result = ActionInputsSchema.parse(input);
			expect(result.coderTemplatePreset).toBe(input.coderTemplatePreset);
		});

		test("accepts valid URL formats", () => {
			const validUrls = [
				"https://coder.test",
				"https://coder.example.com:8080",
				"http://12.34.56.78",
				"https://12.34.56.78:9000",
				"http://localhost:3000",
				"http://127.0.0.1:3000",
				"http://[::1]:3000",
			];

			for (const url of validUrls) {
				const input: ActionInputs = {
					...actionInputValid,
					coderURL: url,
				};
				const result = ActionInputsSchema.parse(input);
				expect(result.coderURL).toBe(url);
			}
		});
	});

	describe("Invalid Input Cases", () => {
		test("rejects missing required fields", () => {
			const input = {} as ActionInputs;
			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});

		test("rejects invalid URL format for coderUrl", () => {
			const input: ActionInputs = {
				...actionInputValid,
				coderURL: "not-a-url",
			};
			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});

		test("rejects invalid URL format for issueUrl", () => {
			const input: ActionInputs = {
				...actionInputValid,
				githubIssueURL: "not-a-url",
			};
			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});

		test("rejects empty strings for required fields", () => {
			const input: ActionInputs = {
				...actionInputValid,
				coderToken: "",
			};
			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});
	});
});
