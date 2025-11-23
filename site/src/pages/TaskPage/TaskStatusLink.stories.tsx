import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskStatusLink } from "./TaskStatusLink";

const meta: Meta<typeof TaskStatusLink> = {
	title: "pages/TaskPage/TaskStatusLink",
	component: TaskStatusLink,
	// Add a wrapper to test truncation.
	decorators: [
		(Story) => (
			<div style={{ display: "flex", width: "200px" }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof TaskStatusLink>;

export const GitHubNewPR: Story = {
	args: {
		uri: "https://github.com/coder/coder/pull/new/fix-deleted-template-button",
	},
};

export const GithubPRNumber: Story = {
	args: {
		uri: "https://github.com/org/repo/pull/1234",
	},
};

export const GitHubPRNoNumber: Story = {
	args: {
		uri: "https://github.com/org/repo/pull",
	},
};

export const GitHubNewIssue: Story = {
	args: {
		uri: "https://github.com/coder/coder/issues/new?template=BLANK_ISSUE",
	},
};

export const GithubIssueNumber: Story = {
	args: {
		uri: "https://github.com/org/repo/issues/4321",
	},
};

export const GithubIssueNoNumber: Story = {
	args: {
		uri: "https://github.com/org/repo/issues",
	},
};

export const GithubOrgRepo: Story = {
	args: {
		uri: "https://github.com/org/repo",
	},
};

export const GithubOrg: Story = {
	args: {
		uri: "https://github.com/org",
	},
};

export const Github: Story = {
	args: {
		uri: "https://github.com",
	},
};

export const File: Story = {
	args: {
		uri: "file:///path/to/file",
	},
};

export const Long: Story = {
	args: {
		uri: "https://dev.coder.com/this-is-a/long-url/to-test/how-the-truncation/looks",
	},
};

export const InvalidPathNotRendered: Story = {
	args: {
		uri: "/path/to/foo",
	},
};

export const InvalidRelativePathNotRendered: Story = {
	args: {
		uri: "path/to/foo",
	},
};
