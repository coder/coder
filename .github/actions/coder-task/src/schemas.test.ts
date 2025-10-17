import { describe, expect, test } from "bun:test";
import {
	ActionInputsSchema,
	UserSchema,
	TemplateSchema,
	TaskStatusSchema,
	CreateTaskParamsSchema,
} from "./schemas";
import { mockUser, mockTemplate, mockTaskStatus } from "./test-helpers";

describe("ActionInputsSchema", () => {
	describe("Valid Input Cases", () => {
		test("accepts minimal required inputs", () => {
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
			};

			const result = ActionInputsSchema.parse(input);

			expect(result.coderUrl).toBe(input.coderUrl);
			expect(result.coderToken).toBe(input.coderToken);
			expect(result.templateName).toBe(input.templateName);
			expect(result.taskPrompt).toBe(input.taskPrompt);
			expect(result.githubToken).toBe(input.githubToken);
		});

		test("applies default values correctly", () => {
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
			};

			const result = ActionInputsSchema.parse(input);

			expect(result.templatePreset).toBe("Default");
			expect(result.taskNamePrefix).toBe("task");
			expect(result.organization).toBe("coder");
			expect(result.commentOnIssue).toBe(true);
		});

		test("accepts all optional inputs", () => {
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
				githubUserId: 12345,
				githubUsername: "testuser",
				templatePreset: "Custom",
				taskNamePrefix: "custom-task",
				taskName: "custom-name",
				organization: "custom-org",
				issueUrl: "https://github.com/owner/repo/issues/123",
				commentOnIssue: false,
				coderWebUrl: "https://coder-web.test",
			};

			const result = ActionInputsSchema.parse(input);

			expect(result.githubUserId).toBe(12345);
			expect(result.githubUsername).toBe("testuser");
			expect(result.templatePreset).toBe("Custom");
			expect(result.taskNamePrefix).toBe("custom-task");
			expect(result.taskName).toBe("custom-name");
			expect(result.organization).toBe("custom-org");
			expect(result.issueUrl).toBe(input.issueUrl);
			expect(result.commentOnIssue).toBe(false);
			expect(result.coderWebUrl).toBe(input.coderWebUrl);
		});

		test("validates URL formats correctly", () => {
			const validUrls = [
				"https://coder.test",
				"http://localhost:3000",
				"https://coder.example.com:8080",
			];

			for (const url of validUrls) {
				const input = {
					coderUrl: url,
					coderToken: "test-token",
					templateName: "test-template",
					taskPrompt: "test prompt",
					githubToken: "github-token",
				};

				const result = ActionInputsSchema.parse(input);
				expect(result.coderUrl).toBe(url);
			}
		});

		test("parses boolean from string for commentOnIssue", () => {
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
				commentOnIssue: "false" as any,
			};

			const result = ActionInputsSchema.parse(input);
			expect(result.commentOnIssue).toBe(false);
		});
	});

	describe("Invalid Input Cases", () => {
		test("rejects missing required fields", () => {
			const input = {
				coderUrl: "https://coder.test",
				// Missing coderToken, templateName, taskPrompt, githubToken
			};

			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});

		test("rejects invalid URL format for coderUrl", () => {
			const input = {
				coderUrl: "not-a-url",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
			};

			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});

		test("rejects invalid URL format for issueUrl", () => {
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
				issueUrl: "not-a-url",
			};

			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});

		test("rejects empty strings for required fields", () => {
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
			};

			expect(() => ActionInputsSchema.parse(input)).toThrow();
		});
	});

	describe("Edge Cases", () => {
		test("handles URL with special characters", () => {
			const input = {
				coderUrl: "https://coder.test?param=value&other=123",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: "test prompt",
				githubToken: "github-token",
			};

			const result = ActionInputsSchema.parse(input);
			expect(result.coderUrl).toBe(input.coderUrl);
		});

		test("handles very long prompt text", () => {
			const longPrompt = "x".repeat(50000); // 50KB prompt
			const input = {
				coderUrl: "https://coder.test",
				coderToken: "test-token",
				templateName: "test-template",
				taskPrompt: longPrompt,
				githubToken: "github-token",
			};

			const result = ActionInputsSchema.parse(input);
			expect(result.taskPrompt).toBe(longPrompt);
		});
	});
});

describe("UserSchema", () => {
	test("parses valid user response", () => {
		const result = UserSchema.parse(mockUser);

		expect(result.id).toBe(mockUser.id);
		expect(result.username).toBe(mockUser.username);
		expect(result.email).toBe(mockUser.email);
		expect(result.github_com_user_id).toBe(mockUser.github_com_user_id);
	});

	test("rejects missing required fields", () => {
		const invalidUser = {
			id: "550e8400-e29b-41d4-a716-446655440000",
			// Missing username and other required fields
		};

		expect(() => UserSchema.parse(invalidUser)).toThrow();
	});

	test("validates UUID format for id", () => {
		const invalidUser = {
			...mockUser,
			id: "not-a-uuid",
		};

		expect(() => UserSchema.parse(invalidUser)).toThrow();
	});
});

describe("TemplateSchema", () => {
	test("parses valid template response", () => {
		const result = TemplateSchema.parse(mockTemplate);

		expect(result.id).toBe(mockTemplate.id);
		expect(result.name).toBe(mockTemplate.name);
		expect(result.organization_id).toBe(mockTemplate.organization_id);
	});

	test("rejects missing required fields", () => {
		const invalidTemplate = {
			id: "770e8400-e29b-41d4-a716-446655440000",
			// Missing name and other required fields
		};

		expect(() => TemplateSchema.parse(invalidTemplate)).toThrow();
	});

	test("validates UUID format for id", () => {
		const invalidTemplate = {
			...mockTemplate,
			id: "not-a-uuid",
		};

		expect(() => TemplateSchema.parse(invalidTemplate)).toThrow();
	});
});

describe("TaskStatusSchema", () => {
	test("parses valid task status response", () => {
		const result = TaskStatusSchema.parse(mockTaskStatus);

		expect(result.id).toBe(mockTaskStatus.id);
		expect(result.name).toBe(mockTaskStatus.name);
		expect(result.status).toBe(mockTaskStatus.status);
	});

	test("rejects missing required fields", () => {
		const invalidTaskStatus = {
			id: "990e8400-e29b-41d4-a716-446655440000",
			// Missing name and other required fields
		};

		expect(() => TaskStatusSchema.parse(invalidTaskStatus)).toThrow();
	});

	test("validates UUID format for id", () => {
		const invalidTaskStatus = {
			...mockTaskStatus,
			id: "not-a-uuid",
		};

		expect(() => TaskStatusSchema.parse(invalidTaskStatus)).toThrow();
	});
});

describe("CreateTaskParamsSchema", () => {
	test("parses valid create task params", () => {
		const params = {
			name: "test-task",
			owner: "testuser",
			templateName: "test-template",
			templatePreset: "Default",
			prompt: "test prompt",
			organization: "coder",
		};

		const result = CreateTaskParamsSchema.parse(params);

		expect(result.name).toBe(params.name);
		expect(result.owner).toBe(params.owner);
		expect(result.templateName).toBe(params.templateName);
		expect(result.templatePreset).toBe(params.templatePreset);
		expect(result.prompt).toBe(params.prompt);
		expect(result.organization).toBe(params.organization);
	});

	test("rejects missing required fields", () => {
		const params = {
			name: "test-task",
			// Missing required fields
		};

		expect(() => CreateTaskParamsSchema.parse(params)).toThrow();
	});

	test("rejects empty strings", () => {
		const params = {
			name: "",
			owner: "testuser",
			templateName: "test-template",
			templatePreset: "Default",
			prompt: "test prompt",
			organization: "coder",
		};

		expect(() => CreateTaskParamsSchema.parse(params)).toThrow();
	});
});
