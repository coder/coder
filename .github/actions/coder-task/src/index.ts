import * as core from "@actions/core";
import * as github from "@actions/github";
import { CoderTaskAction } from "./action";
import { CoderClient } from "./coder-client";
import { ActionInputsSchema } from "./schemas";

async function main() {
	try {
		// Parse and validate inputs
		const inputs = ActionInputsSchema.parse({
			coderUrl: core.getInput("coder-url", { required: true }),
			coderToken: core.getInput("coder-token", { required: true }),
			templateName: core.getInput("template-name", { required: true }),
			taskPrompt: core.getInput("task-prompt", { required: true }),
			githubUserId: core.getInput("github-user-id")
				? parseInt(core.getInput("github-user-id"), 10)
				: undefined,
			githubUsername: core.getInput("github-username") || undefined,
			templatePreset: core.getInput("template-preset") || "Default",
			taskNamePrefix: core.getInput("task-name-prefix") || "task",
			taskName: core.getInput("task-name") || undefined,
			organization: core.getInput("organization") || "coder",
			issueUrl: core.getInput("issue-url") || undefined,
			commentOnIssue: core.getBooleanInput("comment-on-issue") !== false,
			coderWebUrl: core.getInput("coder-web-url") || undefined,
			githubToken: core.getInput("github-token", { required: true }),
		});

		console.log("Inputs validated successfully");
		console.log(`Coder URL: ${inputs.coderUrl}`);
		console.log(`Template: ${inputs.templateName}`);
		console.log(`Organization: ${inputs.organization}`);

		// Initialize clients
		const coder = new CoderClient(inputs.coderUrl, inputs.coderToken);
		const octokit = github.getOctokit(inputs.githubToken);

		console.log("Clients initialized");

		// Execute action
		const action = new CoderTaskAction(coder, octokit, inputs);
		const outputs = await action.run();

		// Set outputs
		core.setOutput("coder-username", outputs.coderUsername);
		core.setOutput("task-name", outputs.taskName);
		core.setOutput("task-url", outputs.taskUrl);
		core.setOutput("task-exists", outputs.taskExists.toString());

		console.log("Action completed successfully");
		console.log(`Outputs: ${JSON.stringify(outputs, null, 2)}`);
	} catch (error) {
		if (error instanceof Error) {
			core.setFailed(error.message);
			console.error("Action failed:", error);
			if (error.stack) {
				console.error("Stack trace:", error.stack);
			}
		} else {
			core.setFailed("Unknown error occurred");
			console.error("Unknown error:", error);
		}
		process.exit(1);
	}
}

main();
