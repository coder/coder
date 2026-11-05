import type { Automation } from "./AgentAutomationsPageView";

/** Temporary rows used until the Automations API is available. */
export const placeholderAutomations: readonly Automation[] = [
	{
		id: "daily-pr-review",
		name: "Daily PR review",
		prompt:
			"Review open pull requests and summarize anything that needs attention.",
		owner: { username: "testuser", name: "TestUser" },
		trigger: {
			type: "timetable",
			label: "Every 5 minutes",
			timezone: "UTC",
		},
		enabled: true,
	},
	{
		id: "weekly-dependency-check",
		name: "Weekly dependency check",
		prompt: "Check for outdated dependencies and flag security updates.",
		owner: {
			username: "alex-chen",
			name: "Alexandra Chen-Washington",
			avatar_url: "/emojis/1f9d1-200d-1f4bb.png",
		},
		trigger: {
			type: "timetable",
			label: "Weekly on Monday at 09:00",
			timezone: "Europe/Berlin",
		},
		enabled: true,
	},
	{
		id: "release-notes",
		name: "Release notes",
		prompt: "Draft release notes from changes merged this week.",
		owner: { username: "testuser", name: "TestUser" },
		trigger: { type: "webhook", usage: "Multi-use" },
		enabled: false,
	},
];
