import { readFileSync } from "node:fs";
import type { CustomProjectConfig } from "@coder/pixel-storybook";

// Resolves the commit hash from the GitHub Actions event payload. For pull
// request events, this prefers the merge commit SHA or the PR head SHA over
// GITHUB_SHA, which points at the merge ref and can be stale.
function resolveCommitHash(): string | undefined {
	if (process.env["GITHUB_EVENT_NAME"] === "pull_request") {
		const eventPath = process.env["GITHUB_EVENT_PATH"];
		if (eventPath) {
			try {
				const event = JSON.parse(readFileSync(eventPath, "utf-8"));
				if (event.after) return event.after;
				if (event.pull_request?.head?.sha) return event.pull_request.head.sha;
			} catch {
				// Fall through to GITHUB_SHA.
			}
		}
	}
	return process.env["GITHUB_SHA"];
}

export const config: CustomProjectConfig = {
	lostPixelPlatform: "https://pixel.coder.com/api",

	storybookShots: {
		storybookUrl: "storybook-static/",
	},

	lostPixelProjectId: "019eb90c-3c26-70ee-8e97-230f1b0388b9",
	apiKey: process.env["PIXEL_KEY"],

	// CI context, populated automatically in GitHub Actions.
	ciBuildId: process.env["GITHUB_RUN_ID"],
	ciBuildNumber: process.env["GITHUB_RUN_NUMBER"],
	repository: process.env["GITHUB_REPOSITORY"],
	commitRefName:
		process.env["GITHUB_EVENT_NAME"] === "pull_request"
			? process.env["GITHUB_HEAD_REF"]
			: process.env["GITHUB_REF_NAME"],
	commitHash: resolveCommitHash(),

	// Browser configuration.
	// browser: "chromium", // defaults to chromium
	// threshold: 0, // defaults to 0. how is this supposed to work in platform mode?
	// waitBeforeScreenshot: 1000,
	shotConcurrency: 64,
};

// Leaving this here so it's easy to migrate to next week when I publish the
// new version of the engine with JSON config.
// Generated pixel.json:
// {
//   "projectId": "019eb90c-3c26-70ee-8e97-230f1b0388b9",
//   "platformAccessUrl": "https://pixel.coder.com",
//   "storybook": {
//     "buildCommand": "pnpm exec storybook build --test --disable-telemetry --stats-json storybook-static/",
//     "outputDirectory": "storybook-static/"
//   },
//   "matrix": {
//     "browsers": ["chrome", "safari", "firefox"],
//     "viewports": ["mobile", "tablet", "desktop"],
//     "themes": ["light", "dark"]
//   }
// }
