import * as core from "@actions/core";
import type { CoderClient } from "./coder-client";
import type { ActionInputs, ActionOutputs } from "./schemas";
import type { getOctokit } from "@actions/github";

export type Octokit = ReturnType<typeof getOctokit>;

export class CoderTaskAction {
	constructor(
		private readonly coder: CoderClient,
		private readonly octokit: Octokit,
		private readonly inputs: ActionInputs,
	) {}

	/**
	 * Resolve GitHub user ID from inputs or context
	 */
	async resolveGitHubUserId(): Promise<number> {
		if (this.inputs.githubUserId) {
			return this.inputs.githubUserId;
		}

		if (this.inputs.githubUsername) {
			const { data: user } = await this.octokit.rest.users.getByUsername({
				username: this.inputs.githubUsername,
			});
			return user.id;
		}

		throw new Error("Either githubUserId or githubUsername must be provided");
	}

	/**
	 * Generate task name based on inputs
	 */
	generateTaskName(issueNumber?: number): string {
		if (this.inputs.taskName) {
			return this.inputs.taskName;
		}

		const contextKey = issueNumber ? `gh-${issueNumber}` : `run-${Date.now()}`;
		return `${this.inputs.taskNamePrefix}-${contextKey}`;
	}

	/**
	 * Extract issue number from issue URL
	 */
	async getIssueNumber(): Promise<number | undefined> {
		if (!this.inputs.issueUrl) {
			return undefined;
		}

		try {
			// Parse issue URL: https://github.com/owner/repo/issues/123
			const urlParts = this.inputs.issueUrl.split("/");
			const issueNumber = Number.parseInt(urlParts[urlParts.length - 1], 10);
			if (!Number.isNaN(issueNumber)) {
				return issueNumber;
			}
		} catch (error) {
			console.warn("Failed to parse issue number from URL:", error);
		}

		return undefined;
	}

	/**
	 * Comment on GitHub issue with task link
	 */
	async commentOnIssue(
		taskUrl: string,
		owner: string,
		repo: string,
		issueNumber: number,
	): Promise<void> {
		const body = `Task created: ${taskUrl}`;

		try {
			// Try to find existing comment from bot
			const { data: comments } = await this.octokit.rest.issues.listComments({
				owner,
				repo,
				issue_number: issueNumber,
			});

			// Find the last comment that starts with "Task created:"
			const existingComment = comments
				.reverse()
				.find((comment: { body?: string }) =>
					comment.body?.startsWith("Task created:"),
				);

			if (existingComment) {
				// Update existing comment
				await this.octokit.rest.issues.updateComment({
					owner,
					repo,
					comment_id: existingComment.id,
					body,
				});
			} else {
				// Create new comment
				await this.octokit.rest.issues.createComment({
					owner,
					repo,
					issue_number: issueNumber,
					body,
				});
			}
		} catch (error) {
			console.warn("Failed to comment on issue:", error);
			// Don't fail the action if commenting fails
		}
	}

	/**
	 * Parse owner and repo from issue URL
	 */
	parseIssueUrl(): {
		owner: string;
		repo: string;
		issueNumber: number;
	} | null {
		if (!this.inputs.issueUrl) {
			return null;
		}

		try {
			// Parse: https://github.com/owner/repo/issues/123
			const url = new URL(this.inputs.issueUrl);
			const pathParts = url.pathname.split("/").filter(Boolean);

			if (
				url.hostname === "github.com" &&
				pathParts.length >= 4 &&
				pathParts[2] === "issues"
			) {
				return {
					owner: pathParts[0],
					repo: pathParts[1],
					issueNumber: Number.parseInt(pathParts[3], 10),
				};
			}
		} catch (error) {
			console.warn("Failed to parse issue URL:", error);
		}

		return null;
	}

	/**
	 * Generate task URL
	 */
	generateTaskUrl(coderUsername: string, taskName: string): string {
		const webUrl = this.inputs.coderWebUrl || this.inputs.coderUrl;
		return `${webUrl}/tasks/${coderUsername}/${taskName}`;
	}

	/**
	 * Main action execution
	 */
	async run(): Promise<ActionOutputs> {
		// 1. Resolve GitHub user ID
		const githubUserId = await this.resolveGitHubUserId();
		core.debug(`GitHub user ID: ${githubUserId}`);

		// 2. Get Coder username from GitHub ID
		const coderUser = await this.coder.getCoderUserByGitHubId(githubUserId);
		core.debug(`Coder username: ${coderUser.username}`);

		// 3. Generate task name
		const issueNumber = await this.getIssueNumber();
		const taskName = this.generateTaskName(issueNumber);
		core.debug(`Task name: ${taskName}`);

		// 4. Check if task already exists
		const existingTask = await this.coder.getTask(coderUser.username, taskName);

		if (existingTask) {
			core.debug(`Task already exists: ${existingTask.id}`);
			core.debug("Sending prompt to existing task...");

			// Send prompt to existing task
			await this.coder.sendTaskInput(
				coderUser.username,
				taskName,
				this.inputs.taskPrompt,
			);
			core.debug("Prompt sent successfully");
		} else {
			core.debug("Creating new task...");

			// Create new task
			await this.coder.createTask({
				name: taskName,
				owner: coderUser.username,
				templateName: this.inputs.templateName,
				templatePreset: this.inputs.templatePreset,
				prompt: this.inputs.taskPrompt,
				organization: this.inputs.organization,
			});
			core.debug("Task created successfully");
		}

		// 5. Generate task URL
		const taskUrl = this.generateTaskUrl(coderUser.username, taskName);
		core.debug(`Task URL: ${taskUrl}`);

		// 6. Comment on issue if requested
		if (this.inputs.issueUrl && this.inputs.commentOnIssue) {
			const issueInfo = this.parseIssueUrl();
			if (issueInfo) {
				core.debug("Commenting on issue...");
				await this.commentOnIssue(
					taskUrl,
					issueInfo.owner,
					issueInfo.repo,
					issueInfo.issueNumber,
				);
				core.debug("Comment posted successfully");
			}
		}

		// Return outputs
		return {
			coderUsername: coderUser.username,
			taskName: `${coderUser.username}/${taskName}`,
			taskUrl,
			taskExists: !!existingTask,
		};
	}
}
