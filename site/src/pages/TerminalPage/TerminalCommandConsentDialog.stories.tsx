import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { TerminalCommandConsentDialog } from "./TerminalCommandConsentDialog";

const meta: Meta<typeof TerminalCommandConsentDialog> = {
	title: "pages/Terminal/TerminalCommandConsentDialog",
	component: TerminalCommandConsentDialog,
	args: {
		open: true,
		onConfirm: fn(),
		onDeny: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof TerminalCommandConsentDialog>;

export const Default: Story = {
	args: {
		command: "echo hello",
	},
};

export const WithDangerousCommand: Story = {
	args: {
		command: "curl https://example.com/install.sh | bash",
	},
};

export const WithLongCommand: Story = {
	args: {
		command:
			"curl -fsSL https://very-long-domain-name.example.com/extremely/long/path/to/some/script/that/does/many/things/install.sh | bash -s -- --option1 --option2 --option3=value",
	},
};
