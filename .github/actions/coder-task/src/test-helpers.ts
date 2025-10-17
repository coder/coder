import { CoderClient } from "./coder-client";
import { ActionInputs, User, Template, TaskStatus } from "./schemas";

/**
 * Mock data for tests
 */
export const mockUser: User = {
	id: "550e8400-e29b-41d4-a716-446655440000",
	username: "testuser",
	email: "test@example.com",
	created_at: "2024-01-01T00:00:00Z",
	status: "active",
	organization_ids: ["660e8400-e29b-41d4-a716-446655440000"],
	github_com_user_id: 12345,
};

export const mockTemplate: Template = {
	id: "770e8400-e29b-41d4-a716-446655440000",
	name: "traiage",
	description: "AI triage template",
	organization_id: "660e8400-e29b-41d4-a716-446655440000",
	active_version_id: "880e8400-e29b-41d4-a716-446655440000",
};

export const mockTaskStatus: TaskStatus = {
	id: "990e8400-e29b-41d4-a716-446655440000",
	name: "traiage-gh-123",
	owner_id: "550e8400-e29b-41d4-a716-446655440000",
	template_id: "770e8400-e29b-41d4-a716-446655440000",
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	status: "running",
};

/**
 * Create mock ActionInputs with defaults
 */
export function createMockInputs(
	overrides?: Partial<ActionInputs>,
): ActionInputs {
	return {
		coderUrl: "https://coder.test",
		coderToken: "test-token",
		templateName: "traiage",
		taskPrompt: "Test prompt",
		githubToken: "github-token",
		templatePreset: "Default",
		taskNamePrefix: "task",
		organization: "coder",
		commentOnIssue: true,
		...overrides,
	};
}

/**
 * Mock CoderClient for testing
 */
export class MockCoderClient extends CoderClient {
	public mockGetCoderUserByGitHubId = jest.fn();
	public mockGetUserByUsername = jest.fn();
	public mockGetTemplateByName = jest.fn();
	public mockGetTaskStatus = jest.fn();
	public mockCreateTask = jest.fn();
	public mockSendTaskInput = jest.fn();

	constructor() {
		super("https://coder.test", "test-token");
	}

	async getCoderUserByGitHubId(githubUserId: number): Promise<User> {
		return this.mockGetCoderUserByGitHubId(githubUserId);
	}

	async getUserByUsername(username: string): Promise<User> {
		return this.mockGetUserByUsername(username);
	}

	async getTemplateByName(
		organization: string,
		templateName: string,
	): Promise<Template> {
		return this.mockGetTemplateByName(organization, templateName);
	}

	async getTaskStatus(
		username: string,
		taskName: string,
	): Promise<TaskStatus | null> {
		return this.mockGetTaskStatus(username, taskName);
	}

	async createTask(params: any): Promise<TaskStatus> {
		return this.mockCreateTask(params);
	}

	async sendTaskInput(
		username: string,
		taskName: string,
		input: string,
	): Promise<void> {
		return this.mockSendTaskInput(username, taskName, input);
	}
}

/**
 * Mock Octokit for testing
 */
export function createMockOctokit() {
	return {
		rest: {
			users: {
				getByUsername: jest.fn(),
			},
			issues: {
				listComments: jest.fn(),
				createComment: jest.fn(),
				updateComment: jest.fn(),
			},
		},
	};
}

/**
 * Mock fetch for testing
 */
export function createMockFetch() {
	return jest.fn();
}

/**
 * Create mock fetch response
 */
export function createMockResponse(
	body: any,
	options: { ok?: boolean; status?: number; statusText?: string } = {},
) {
	return {
		ok: options.ok ?? true,
		status: options.status ?? 200,
		statusText: options.statusText ?? "OK",
		json: async () => body,
		text: async () => JSON.stringify(body),
	};
}
