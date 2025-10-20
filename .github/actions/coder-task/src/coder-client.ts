import { z } from "zod";
import {
  User,
  UserSchema,
  TaskStatus,
  TaskStatusSchema,
  Template,
  TemplateSchema,
  CreateTaskParams,
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
   * Get Coder user by GitHub user ID
   */
  async getCoderUserByGitHubId(githubUserId: number): Promise<User> {
    const endpoint = `/api/v2/users?q=github_com_user_id:${githubUserId}`;
    const response = await this.request<unknown[]>(endpoint);

    if (!Array.isArray(response) || response.length === 0) {
      throw new CoderAPIError(
        `No Coder user found with GitHub user ID ${githubUserId}`,
        404,
      );
    }

    return UserSchema.parse(response[0]);
  }

  /**
   * Get user by username
   */
  async getUserByUsername(username: string): Promise<User> {
    const endpoint = `/api/v2/users/${username}`;
    const response = await this.request<unknown>(endpoint);
    return UserSchema.parse(response);
  }

  /**
   * Get template by name
   */
  async getTemplateByName(templateName: string): Promise<Template> {
    const endpoint = `/api/v2/templates?q=exact_name:${encodeURIComponent(templateName)}`;
    const response = await this.request<unknown[]>(endpoint);

    if (!Array.isArray(response) || response.length === 0) {
      throw new CoderAPIError(`Template "${templateName}" not found`, 404);
    }

    return TemplateSchema.parse(response[0]);
  }

  /**
   * Check if task exists and get its status
   */
  async getTaskStatus(
    ownerUsername: string,
    taskName: string,
  ): Promise<TaskStatus | null> {
    try {
      const endpoint = `/api/v2/users/${ownerUsername}/tasks/${taskName}`;
      const response = await this.request<unknown>(endpoint);
      return TaskStatusSchema.parse(response);
    } catch (error) {
      if (error instanceof CoderAPIError && error.statusCode === 404) {
        return null;
      }
      throw error;
    }
  }

  /**
   * Create a new task
   */
  async createTask(params: CreateTaskParams): Promise<TaskStatus> {
    // First, get the template to get its ID
    const template = await this.getTemplateByName(params.templateName);

    // Get the owner user to get their ID
    const owner = await this.getUserByUsername(params.owner);

    // Create the task
    const endpoint = `/api/v2/organizations/${params.organization}/members/${params.owner}/tasks`;
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

    return TaskStatusSchema.parse(response);
  }

  /**
   * Send input/prompt to an existing task
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
   * Get task logs
   */
  async getTaskLogs(
    ownerUsername: string,
    taskName: string,
  ): Promise<unknown> {
    const endpoint = `/api/v2/users/${ownerUsername}/tasks/${taskName}/logs`;
    return this.request<unknown>(endpoint);
  }
}
