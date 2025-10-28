import { mock } from "bun:test";
import { CoderClient } from "./coder-client";
import type {
	CoderSDKUser,
	CoderSDKGetUsersResponse,
	CoderSDKTemplate,
	CoderSDKTemplateVersionPreset,
	ExperimentalCoderSDKTask,
	ExperimentalCoderSDKTaskListResponse,
	ExperimentalCoderSDKCreateTaskRequest,
} from "./coder-client";
import type { ActionInputs } from "./schemas";

/**
 * Mock data for tests
 */
export const mockUser: CoderSDKUser = {
	id: "550e8400-e29b-41d4-a716-446655440000",
	username: "testuser",
	email: "test@example.com",
	organization_ids: ["660e8400-e29b-41d4-a716-446655440000"],
	github_com_user_id: 12345,
};

export const mockUserList: CoderSDKGetUsersResponse = {
	users: [mockUser],
};

export const mockUserListEmpty: CoderSDKGetUsersResponse = {
	users: [],
};

export const mockUserListDuplicate: CoderSDKGetUsersResponse = {
	users: [
		mockUser,
		{
			...mockUser,
			id: "660e8400-e29b-41d4-a716-446655440001",
			username: "testuser2",
		},
	],
};

export const mockTemplate: CoderSDKTemplate = {
	id: "770e8400-e29b-41d4-a716-446655440000",
	name: "my-template",
	description: "AI triage template",
	organization_id: "660e8400-e29b-41d4-a716-446655440000",
	active_version_id: "880e8400-e29b-41d4-a716-446655440000",
};

export const mockTemplateVersionPreset: CoderSDKTemplateVersionPreset = {
	ID: "880e8400-e29b-41d4-a716-446655440000",
	Name: "default-preset",
	Default: true,
};

export const mockTemplateVersionPreset2: CoderSDKTemplateVersionPreset = {
	ID: "990e8400-e29b-41d4-a716-446655440000",
	Name: "another-preset",
	Default: false,
};

export const mockTemplateVersionPresets = [
	mockTemplateVersionPreset,
	mockTemplateVersionPreset2,
];

export const mockTask: ExperimentalCoderSDKTask = {
	id: "990e8400-e29b-41d4-a716-446655440000",
	name: "task-123",
	owner_id: "550e8400-e29b-41d4-a716-446655440000",
	template_id: "770e8400-e29b-41d4-a716-446655440000",
	created_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:00Z",
	status: "running",
};

export const mockTaskList: ExperimentalCoderSDKTaskListResponse = {
	tasks: [mockTask],
};

export const mockTaskListEmpty: ExperimentalCoderSDKTaskListResponse = {
	tasks: [],
};

/**
 * Create mock ActionInputs with defaults
 */
export function createMockInputs(
	overrides?: Partial<ActionInputs>,
): ActionInputs {
	return {
		coderTaskPrompt: "Test prompt",
		coderToken: "test-token",
		coderURL: "https://coder.test",
		coderOrganization: "coder",
		coderTaskNamePrefix: "task",
		coderTemplateName: "my-template",
		githubToken: "github-token",
		githubIssueURL: "https://github.com/test-org/test-repo/issues/12345",
		githubUserID: 12345,
		...overrides,
	};
}

/**
 * Mock CoderClient for testing
 */
export class MockCoderClient implements CoderClient {
	private readonly headers: Record<string, string>;
	public mockGetCoderUserByGithubID = mock();
	public mockGetTemplateByOrganizationAndName = mock();
	public mockGetTemplateVersionPresets = mock();
	public mockGetTask = mock();
	public mockCreateTask = mock();
	public mockSendTaskInput = mock();

	constructor() // private readonly serverURL: string,
	// apiToken: string,
	{
		this.headers = {};
	}

	async getCoderUserByGitHubId(githubUserId: number): Promise<CoderSDKUser> {
		return this.mockGetCoderUserByGithubID(githubUserId);
	}

	async getTemplateByOrganizationAndName(
		organization: string,
		templateName: string,
	): Promise<CoderSDKTemplate> {
		return this.mockGetTemplateByOrganizationAndName(
			organization,
			templateName,
		);
	}

	async getTemplateVersionPresets(
		templateVersionId: string,
	): Promise<CoderSDKTemplateVersionPreset[]> {
		return this.mockGetTemplateVersionPresets(templateVersionId);
	}

	async getTask(
		username: string,
		taskName: string,
	): Promise<ExperimentalCoderSDKTask | null> {
		return this.mockGetTask(username, taskName);
	}

	async createTask(
		username: string,
		params: ExperimentalCoderSDKCreateTaskRequest,
	): Promise<ExperimentalCoderSDKTask> {
		return this.mockCreateTask(username, params);
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
	body: unknown,
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
