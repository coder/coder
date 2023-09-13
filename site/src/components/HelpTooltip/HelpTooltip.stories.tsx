import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof HelpTooltip> = {
  title: "components/HelpTooltip",
  component: HelpTooltip,
  args: {
    children: (
      <>
        <HelpTooltipTitle>What is a template?</HelpTooltipTitle>
        <HelpTooltipText>
          A template is a common configuration for your team&apos;s workspaces.
        </HelpTooltipText>
        <HelpTooltipLinksGroup>
          <HelpTooltipLink href="https://github.com/coder/coder/">
            Creating a template
          </HelpTooltipLink>
          <HelpTooltipLink href="https://github.com/coder/coder/">
            Updating a template
          </HelpTooltipLink>
        </HelpTooltipLinksGroup>
      </>
    ),
  },
};

export default meta;
type Story = StoryObj<typeof HelpTooltip>;

export const Example: Story = {};
