import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { SSHKeysPageView } from "./SSHKeysPageView";

const meta: Meta<typeof SSHKeysPageView> = {
	title: "pages/UserSettingsPage/SSHKeysPageView",
	component: SSHKeysPageView,
	args: {
		isLoading: false,
		showSignCommits: false,
		signCommits: false,
		isSignCommitsDisabled: false,
		onSignCommitsChange: fn(),
		sshKey: {
			user_id: "test-user-id",
			created_at: "2022-07-28T07:45:50.795918897Z",
			updated_at: "2022-07-28T07:45:50.795919142Z",
			public_key:
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICnKzATuWwmmt5+CKTPuRGN0R1PBemA+6/SStpLiyX+L",
		},
	},
};

export default meta;
type Story = StoryObj<typeof SSHKeysPageView>;

export const Example: Story = {};

export const SigningAvailable: Story = {
	args: {
		showSignCommits: true,
	},
};

export const SigningEnabled: Story = {
	args: {
		showSignCommits: true,
		signCommits: true,
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
	},
};

export const WithGetSSHKeyError: Story = {
	args: {
		sshKey: undefined,
		getSSHKeyError: mockApiError({
			message: "Failed to get SSH key",
		}),
	},
};
