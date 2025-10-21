import { z } from "zod";

// CoderClient provides a minimal set of methods for interacting with the Coder API.
export class CoderClient {
	private readonly headers: Record<string, string>;

	constructor(
		private readonly serverURL: string,
		apiToken: string,
	) {
		this.headers = {
			"Coder-Session-Token": apiToken,
			"Content-Type": "application/json",
		};
	}

	private async request<T>(
		endpoint: string,
		options?: RequestInit,
	): Promise<T> {
		const url = `${this.serverURL}${endpoint}`;
		const response = await fetch(url, {
			...options,
			headers: { ...this.headers, ...options?.headers },
		});

		if (!response.ok) {
			const body = await response.text().catch(() => "");
			throw new CoderAPIError(
				`Coder API error: ${response.statusText}`,
				response.status,
				body,
			);
		}

		return response.json() as Promise<T>;
	}

	/**
	 * getCoderUserByGitHubId retrieves an existing Coder user with the given GitHub user ID using Coder's stable API.
	 * Throws an error if more than one user exists with the same GitHub user ID or if a GitHub user ID of 0 is provided.
	 */
	async getCoderUserByGitHubId(
		githubUserId: number | undefined,
	): Promise<CoderSDKUser> {
		if (githubUserId === undefined) {
			throw new CoderAPIError("GitHub user ID cannot be undefined", 400);
		}
		if (githubUserId === 0) {
			throw "GitHub user ID cannot be 0";
		}
		const endpoint = `/api/v2/users?q=${encodeURIComponent(`github_com_user_id:"${githubUserId}"`)}`;
		const response = await this.request<unknown[]>(endpoint);
		const userList = CoderSDKGetUsersResponseSchema.parse(response);
		if (userList.users.length === 0) {
			throw new CoderAPIError(
				`No Coder user found with GitHub user ID ${githubUserId}`,
				404,
			);
		}
		if (userList.users.length > 1) {
			throw new CoderAPIError(
				`Multiple Coder users found with GitHub user ID ${githubUserId}`,
				409,
			);
		}
		return CoderSDKUserSchema.parse(userList.users[0]);
	}

	/**
	 * getTemplateByOrganizationAndName retrieves a template via Coder's stable API.
	 */
	async getTemplateByOrganizationAndName(
		organizationName: string,
		templateName: string,
	): Promise<CoderSDKTemplate> {
		const endpoint = `/api/v2/organizations/${encodeURIComponent(organizationName)}/templates/${encodeURIComponent(templateName)}`;
		const response =
			await this.request<typeof CoderSDKTemplateSchema>(endpoint);
		return CoderSDKTemplateSchema.parse(response);
	}

	/**
	 * getTask retrieves an existing task via Coder's experimental Tasks API.
	 * Returns null if the task does not exist.
	 */
	async getTask(
		owner: string,
		taskName: string,
	): Promise<ExperimentalCoderSDKTask | null> {
		// TODO: needs taskByOwnerAndName endpoint, fake it for now with the list endpoint.
		try {
			const allTasksResponse = await this.request<unknown>(
				`/api/experimental/tasks?q=${encodeURIComponent(`owner:${owner}`)}`,
			);
			const allTasks =
				ExperimentalCoderSDKTaskListResponseSchema.parse(allTasksResponse);
			const task = allTasks.tasks.find((t) => t.name === taskName);
			if (!task) {
				return null;
			}
			return task;
		} catch (error) {
			if (error instanceof CoderAPIError && error.statusCode === 404) {
				return null;
			}
			throw error;
		}
	}

	/**
	 * createTask creates a new task with the given parameters using Coder's experimental Tasks API.
	 */
	async createTask(
		params: ExperimentalCoderSDKCreateTaskRequest,
	): Promise<ExperimentalCoderSDKTask> {
		const template = await this.getTemplateByOrganizationAndName(
			params.organization,
			params.templateName,
		);
		const endpoint = `/api/experimental/tasks/${encodeURIComponent(params.owner)}`;
		const body = {
			name: params.name,
			template_id: template.id,
			template_version_preset_id: params.templatePreset,
			prompt: params.prompt,
		};
		const response = await this.request<unknown>(endpoint, {
			method: "POST",
			body: JSON.stringify(body),
		});
		return ExperimentalCoderSDKTaskSchema.parse(response);
	}

	/**
	 * sendTaskInput sends the given input to an existing task via Coder's experimental Tasks API.
	 */
	async sendTaskInput(
		ownerUsername: string,
		taskName: string,
		input: string,
	): Promise<void> {
		const endpoint = `/api/v2/users/${ownerUsername}/tasks/${taskName}/send`;
		await this.request<unknown>(endpoint, {
			method: "POST",
			body: JSON.stringify({ input }),
		});
	}
}

// CoderSDKUserSchema is the schema for codersdk.User.
export const CoderSDKUserSchema = z.object({
	id: z.string().uuid(),
	username: z.string(),
	email: z.string().email(),
	organization_ids: z.array(z.string().uuid()),
	github_com_user_id: z.number().optional(),
});
export type CoderSDKUser = z.infer<typeof CoderSDKUserSchema>;

// CoderSDKUserListSchema is the schema for codersdk.GetUsersResponse.
export const CoderSDKGetUsersResponseSchema = z.object({
	users: z.array(CoderSDKUserSchema),
});
export type CoderSDKGetUsersResponse = z.infer<
	typeof CoderSDKGetUsersResponseSchema
>;

// CoderSDKTemplateSchema is the schema for codersdk.Template.
export const CoderSDKTemplateSchema = z.object({
	id: z.string().uuid(),
	name: z.string(),
	description: z.string().optional(),
	organization_id: z.string().uuid(),
	active_version_id: z.string().uuid(),
});
export type CoderSDKTemplate = z.infer<typeof CoderSDKTemplateSchema>;

// ExperimentalCoderSDKCreateTaskRequestSchema is the schema for experimental codersdk.CreateTaskRequest.
export const ExperimentalCoderSDKCreateTaskRequestSchema = z.object({
	name: z.string().min(1),
	owner: z.string().min(1),
	templateName: z.string().min(1),
	templatePreset: z.string().min(1),
	prompt: z.string().min(1),
	organization: z.string().min(1),
});
export type ExperimentalCoderSDKCreateTaskRequest = z.infer<
	typeof ExperimentalCoderSDKCreateTaskRequestSchema
>;

// ExperimentalCoderSDKTaskSchema is the schema for experimental codersdk.Task.
export const ExperimentalCoderSDKTaskSchema = z.object({
	id: z.string().uuid(),
	name: z.string(),
	owner_id: z.string().uuid(),
	template_id: z.string().uuid(),
	created_at: z.string(),
	updated_at: z.string(),
	status: z.string(),
});
export type ExperimentalCoderSDKTask = z.infer<
	typeof ExperimentalCoderSDKTaskSchema
>;

// ExperimentalCoderSDKTaskListResponseSchema is the schema for Coder's GET /api/experimental/tasks endpoint.
// At the time of writing, this type is not exported by github.com/coder/coder/v2/codersdk.
export const ExperimentalCoderSDKTaskListResponseSchema = z.object({
	tasks: z.array(ExperimentalCoderSDKTaskSchema),
});
export type ExperimentalCoderSDKTaskListResponse = z.infer<
	typeof ExperimentalCoderSDKTaskListResponseSchema
>;

// CoderAPIError is a custom error class for Coder API errors.
export class CoderAPIError extends Error {
	constructor(
		message: string,
		public readonly statusCode: number,
		public readonly response?: unknown,
	) {
		super(message);
		this.name = "CoderAPIError";
	}
}
