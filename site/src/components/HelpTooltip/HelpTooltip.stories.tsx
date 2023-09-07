import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipProps,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip";
import type { Meta, StoryObj } from "@storybook/react";

const Template = (props: HelpTooltipProps) => (
  <HelpTooltip {...props}>
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
  </HelpTooltip>
);

const meta: Meta<typeof HelpTooltip> = {
  title: "components/HelpTooltip/HelpTooltip",
  component: Template,
};

export default meta;
type Story = StoryObj<typeof HelpTooltip>;

export const Close: Story = {};

export const Open: Story = {
  args: {
    open: true,
  },
};
