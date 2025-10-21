import type { User, Task, Template, CreateTaskParams } from "./schemas";
import {
	UserSchema,
	UserListSchema,
	TaskSchema,
	TaskListSchema,
	TemplateSchema,
} from "./schemas";

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
	async getCoderUserByGitHubId(githubUserId: number): Promise<User> {
		if (githubUserId === 0) {
			throw "GitHub user ID cannot be 0";
		}
		const endpoint = `/api/v2/users?q=${encodeURIComponent("github_com_user_id:" + githubUserId)}`;
		const response = await this.request<unknown[]>(endpoint);
		const userList = UserListSchema.parse(response);
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
		return UserSchema.parse(userList.users[0]);
	}

	/**
	 * getTemplateByOrganizationAndName retrieves a template via Coder's stable API.
	 */
	async getTemplateByOrganizationAndName(
		organizationName: string,
		templateName: string,
	): Promise<Template> {
		const endpoint = `/api/v2/organizations/${encodeURIComponent(organizationName)}/templates/${encodeURIComponent(templateName)}`;
		const response = await this.request<typeof TemplateSchema>(endpoint);
		return TemplateSchema.parse(response);
	}

	/**
	 * getTask retrieves an existing task via Coder's experimental Tasks API.
	 * Returns null if the task does not exist.
	 */
	async getTask(owner: string, taskName: string): Promise<Task | null> {
		// TODO: needs taskByOwnerAndName endpoint, fake it for now.
		try {
			const allTasksResponse = await this.request<unknown>(
				`/api/experimental/tasks?q=${encodeURIComponent(`owner:${owner}`)}`,
			);
			const allTasks = TaskListSchema.parse(allTasksResponse);
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
	async createTask(params: CreateTaskParams): Promise<Task> {
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
		return TaskSchema.parse(response);
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

	/**
	 * getTaskLogs retrieves the logs for an existing task via Coder's experimental Tasks API.
	 */
	async getTaskLogs(ownerUsername: string, taskName: string): Promise<unknown> {
		const endpoint = `/api/v2/users/${ownerUsername}/tasks/${taskName}/logs`;
		return this.request<unknown>(endpoint);
	}
}
