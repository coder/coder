import * as core from "@actions/core";
import { ExperimentalCoderSDKCreateTaskRequest, type CoderClient } from "./coder-client";
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
	 * generateTaskName generates a task name in the format `<prefix>-<issue number>`
	 */
	generateTaskName(issueNumber: number): string {
		if (!this.inputs.coderTaskNamePrefix || !this.inputs.githubIssueURL) {
			throw new Error(
				"either taskName or both taskNamePrefix and issueURL must be provided",
			);
		}
		return `${this.inputs.coderTaskNamePrefix}-${issueNumber}`;
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
	parseGithubIssueURL(): {
		githubOrg: string;
		githubRepo: string;
		githubIssueNumber: number;
	} {
		if (!this.inputs.githubIssueURL) {
			throw new Error(`Missing issue URL`);
		}

		// Parse: https://github.com/owner/repo/issues/123
			const match = this.inputs.githubIssueURL.match(/([^/]+)\/([^/]+)\/issues\/(\d+)/);
			if (!match) {
				throw new Error(`Invalid issue URL: ${this.inputs.githubIssueURL}`);
			}
			return {
				githubOrg: match[1],
				githubRepo: match[2],
				githubIssueNumber: parseInt(match[3], 10),
			};
	}

	/**
	 * Generate task URL
	 */
	generateTaskUrl(coderUsername: string, taskName: string): string {
		return `${this.inputs.coderURL}/tasks/${coderUsername}/${taskName}`;
	}

	/**
	 * Main action execution
	 */
	async run(): Promise<ActionOutputs> {
		core.debug(`GitHub user ID: ${this.inputs.githubUserID}`);
		const coderUser = await this.coder.getCoderUserByGitHubId(this.inputs.githubUserID);
		const {githubOrg, githubRepo, githubIssueNumber} = this.parseGithubIssueURL();
		core.debug(`GitHub owner: ${githubOrg}`);
		core.debug(`GitHub repo: ${githubRepo}`);
		core.debug(`GitHub issue number: ${githubIssueNumber}`);
		core.debug(`Coder username: ${coderUser.username}`);
		const taskName = this.generateTaskName(githubIssueNumber);
		core.debug(`Coder Task name: ${taskName}`);
		const template = await this.coder.getTemplateByOrganizationAndName(this.inputs.coderOrganization, this.inputs.coderTemplateName);
		core.debug(`Coder Template ID: ${template.id} (Active version ID: ${template.active_version_id})`);
		const templateVersionPresets = await this.coder.getTemplateVersionPresets(template.active_version_id);
		let presetID = undefined;
		// If no preset specified, use default preset
		if (!this.inputs.coderTemplatePreset) {
			for (const preset of templateVersionPresets) {
				if (preset.Name === this.inputs.coderTemplatePreset) {
					presetID = preset.ID;
					core.debug(`Coder Template Preset ID: ${presetID}`);
					break;
				}
			}
			// User requested a preset that does not exist
			if (this.inputs.coderTemplatePreset && !presetID) {
				throw new Error(`Preset ${this.inputs.coderTemplatePreset} not found`);
			}
		}

		const existingTask = await this.coder.getTask(coderUser.username, taskName);
		if (existingTask) {
			core.debug(`Task already exists: ${existingTask.id}`);
			core.debug("Sending prompt to existing task...");
			// Send prompt to existing task
			await this.coder.sendTaskInput(
				coderUser.username,
				taskName,
				this.inputs.coderTaskPrompt,
			);
			core.debug("Prompt sent successfully");
			return {
				coderUsername: coderUser.username,
				taskName: existingTask.name,
				taskUrl: this.generateTaskUrl(coderUser.username, taskName),
				taskCreated: false
			};
		}
		core.debug("Creating Coder task...");

		const req : ExperimentalCoderSDKCreateTaskRequest = {
			name: taskName,
			template_version_id: this.inputs.coderTemplateName,
			template_version_preset_id: presetID,
			input: this.inputs.coderTaskPrompt,
		}
		// Create new task
		const createdTask  = await this.coder.createTask(coderUser.username, req);
		core.debug("Task created successfully");

		// 5. Generate task URL
		const taskUrl = this.generateTaskUrl(coderUser.username, createdTask.name);
		core.debug(`Task URL: ${taskUrl}`);

		// 6. Comment on issue if requested
		core.debug(`Commenting on issue ${githubOrg}/${githubRepo}#${githubIssueNumber}`);
		await this.commentOnIssue(
			taskUrl,
			githubOrg,
			githubRepo,
			githubIssueNumber,
		);
		core.debug(`Comment posted successfully`);
		return {
			coderUsername: coderUser.username,
			taskName: taskName,
			taskUrl,
			taskCreated: true
		};
}
