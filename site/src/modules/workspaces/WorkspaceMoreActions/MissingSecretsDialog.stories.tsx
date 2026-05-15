import type { Meta, StoryObj } from "@storybook/react-vite";
import { MissingSecretsDialog } from "./MissingSecretsDialog";

const meta: Meta<typeof MissingSecretsDialog> = {
	title: "modules/workspaces/MissingSecretsDialog",
	component: MissingSecretsDialog,
	args: {
		open: true,
		onClose: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof MissingSecretsDialog>;

export const SingleSecret: Story = {
	args: {
		count: 1,
	},
};

export const MultipleSecrets: Story = {
	args: {
		count: 3,
	},
};
