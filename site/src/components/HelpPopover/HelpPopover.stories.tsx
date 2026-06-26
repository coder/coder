import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	HelpPopover,
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
	HelpPopoverTitle,
} from "./HelpPopover";

const meta: Meta<typeof HelpPopover> = {
	title: "components/HelpPopover",
	component: HelpPopover,
	args: {
		children: (
			<>
				<HelpPopoverTitle>What is a template?</HelpPopoverTitle>
				<HelpPopoverText>
					A template is a common configuration for your team&apos;s workspaces.
				</HelpPopoverText>
				<HelpPopoverLinksGroup>
					<HelpPopoverLink href="https://github.com/coder/coder/">
						Creating a template
					</HelpPopoverLink>
					<HelpPopoverLink href="https://github.com/coder/coder/">
						Updating a template
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof HelpPopover>;

export const Example: Story = {};
