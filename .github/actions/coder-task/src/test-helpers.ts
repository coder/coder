import { mock } from "bun:test";
import { CoderClient } from "./coder-client";
import {
	ActionInputs,
	User,
	UserList,
	Template,
	Task,
	TaskList,
} from "./schemas";

/**
 * Mock data for tests
 */
export const mockUser: User = {
	id: "550e8400-e29b-41d4-a716-446655440000",
	username: "testuser",
	email: "test@example.com",
	organization_ids: ["660e8400-e29b-41d4-a716-446655440000"],
	github_com_user_id: 12345,
};

export const mockUserList: UserList = {
	users: [mockUser],
};

export const mockUserListEmpty: UserList = {
	users: [],
};

export const mockUserListDuplicate: UserList = {
	users: [
		mockUser,
		{
			...mockUser,
			id: "660e8400-e29b-41d4-a716-446655440001",
			username: "testuser2",
		},
	],
};

export const mockTemplate: Template = {
	id: "770e8400-e29b-41d4-a716-446655440000",
	name: "my-template",
	description: "AI triage template",
	organization_id: "660e8400-e29b-41d4-a716-446655440000",
	active_version_id: "880e8400-e29b-41d4-a716-446655440000",
};

export const mockTask: Task = {
	id: "990e8400-e29b-41d4-a716-446655440000",
	name: "task-123",
	owner_id: "550e8400-e29b-41d4-a716-446655440000",
	template_id: "770e8400-e29b-41d4-a716-446655440000",
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	status: "running",
};

export const mockTaskList: TaskList = {
	tasks: [mockTask],
};

export const mockTaskListEmpty: TaskList = {
	tasks: [],
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
		templateName: "my-template",
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
	public mockGetCoderUserByGitHubId = mock();
	public mockGetUserByUsername = mock();
	public mockGetTemplateByName = mock();
	public mockGetTaskStatus = mock();
	public mockCreateTask = mock();
	public mockSendTaskInput = mock();

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

	async getTask(username: string, taskName: string): Promise<Task | null> {
		return this.mockGetTaskStatus(username, taskName);
	}

	async createTask(params: any): Promise<Task> {
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
				getByUsername: mock(),
			},
			issues: {
				listComments: mock(),
				createComment: mock(),
				updateComment: mock(),
			},
		},
	};
}

/**
 * Mock fetch for testing
 */
export function createMockFetch() {
	return mock();
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
