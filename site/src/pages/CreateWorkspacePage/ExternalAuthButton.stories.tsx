import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { TemplateVersionExternalAuth } from "#/api/typesGenerated";
import { ExternalAuthButton } from "./ExternalAuthButton";

const MockExternalAuth: TemplateVersionExternalAuth = {
	id: "",
	type: "",
	display_name: "GitHub",
	display_icon: "/icon/github.svg",
	authenticate_url: "",
	authenticated: false,
};

const meta: Meta<typeof ExternalAuthButton> = {
	title: "pages/CreateWorkspacePage/ExternalAuthButton",
	component: ExternalAuthButton,
};

export default meta;
type Story = StoryObj<typeof ExternalAuthButton>;

export const Github: Story = {
	args: {
		auth: MockExternalAuth,
	},
};

export const GithubOptional: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			optional: true,
		},
	},
};

export const GithubWithRetry: Story = {
	args: {
		auth: MockExternalAuth,
		displayRetry: true,
	},
};

export const GithubAuthenticated: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			authenticated: true,
		},
	},
};

export const Gitlab: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			display_icon: "/icon/gitlab.svg",
			display_name: "GitLab",
			authenticated: false,
			optional: true,
		},
	},
};

export const GitlabAuthenticated: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			display_icon: "/icon/gitlab.svg",
			display_name: "GitLab",
			authenticated: true,
		},
	},
};

export const AzureDevOps: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			display_icon: "/icon/azure-devops.svg",
			display_name: "Azure DevOps",
			authenticated: false,
			optional: true,
		},
	},
};

export const AzureDevOpsAuthenticated: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			display_icon: "/icon/azure-devops.svg",
			display_name: "Azure DevOps",
			authenticated: true,
		},
	},
};

export const Bitbucket: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			display_icon: "/icon/bitbucket.svg",
			display_name: "Bitbucket",
			authenticated: false,
			optional: true,
		},
	},
};

export const BitbucketAuthenticated: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			display_icon: "/icon/bitbucket.svg",
			display_name: "Bitbucket",
			authenticated: true,
		},
	},
};

// When an admin creates a workspace for another user, the requester cannot
// authenticate on the owner's behalf, so the login action is hidden and the
// unconnected state is read-only.
export const ForAnotherUserNotConnected: Story = {
	args: {
		auth: MockExternalAuth,
		canAuthenticate: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Not connected")).toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: /login with github/i }),
		).not.toBeInTheDocument();
	},
};

export const ForAnotherUserAuthenticated: Story = {
	args: {
		auth: {
			...MockExternalAuth,
			authenticated: true,
		},
		canAuthenticate: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Authenticated")).toBeInTheDocument();
	},
};
