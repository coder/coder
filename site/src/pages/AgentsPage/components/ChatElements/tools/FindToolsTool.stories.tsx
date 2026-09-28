import type { Meta, StoryObj } from "@storybook/react-vite";
import { FindToolsTool } from "./FindToolsTool";

const githubMatches = [
	{
		name: "github__list_issues",
		description: "List issues in a GitHub repository.",
	},
	{
		name: "github__list_pull_requests",
		description: "List pull requests in a GitHub repository.",
	},
];

const meta: Meta<typeof FindToolsTool> = {
	title: "pages/AgentsPage/ChatElements/tools/FindToolsTool",
	component: FindToolsTool,
	args: {
		queries: ["github issues", "pull requests"],
		names: ["github__list_issues"],
		matches: githubMatches,
		status: "completed",
		isError: false,
	},
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6 font-sans text-xs">
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof FindToolsTool>;

export const Running: Story = {
	args: { status: "running", matches: [] },
};

export const ManyMatches: Story = {};

export const SingleMatch: Story = {
	args: {
		queries: ["github issues"],
		names: [],
		matches: githubMatches.slice(0, 1),
	},
};

export const NoMatches: Story = {
	args: {
		queries: ["nonexistent capability"],
		names: [],
		matches: [],
	},
};

// A long match list overflows the header, so the match count leads the
// label and only the tool names are truncated.
export const LongMatchListTruncates: Story = {
	decorators: [
		(Story) => (
			<div className="w-80">
				<Story />
			</div>
		),
	],
	args: {
		queries: ["microsoft documentation", "code samples"],
		names: [],
		matches: [
			"mslearn__microsoft_docs_search",
			"mslearn__microsoft_docs_fetch",
			"mslearn__microsoft_code_sample_search",
			"mslearn__microsoft_learn_catalog",
		].map((name) => ({ name, description: `${name} description` })),
	},
};

// A long search with no results falls back to showing the search terms.
export const LongSearchNoMatchesTruncates: Story = {
	decorators: [
		(Story) => (
			<div className="w-80">
				<Story />
			</div>
		),
	],
	args: {
		queries: [
			"list github issues for a repository",
			"open pull requests assigned to me",
			"comment on a pull request review thread",
		],
		names: ["github__list_issues"],
		matches: [],
	},
};

export const Failed: Story = {
	args: {
		queries: ["github issues"],
		names: [],
		matches: [],
		status: "error",
		isError: true,
		errorMessage:
			"The schema budget for this step is exhausted; call the tools already activated or retry next step.",
	},
};
