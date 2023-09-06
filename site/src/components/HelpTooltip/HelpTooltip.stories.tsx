import { ComponentMeta, Story } from "@storybook/react";
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipProps,
  HelpTooltipText,
  HelpTooltipTitle,
} from "./HelpTooltip";

export default {
  title: "components/HelpTooltip",
  component: HelpTooltip,
} as ComponentMeta<typeof HelpTooltip>;

const Template: Story<HelpTooltipProps> = (args) => (
  <HelpTooltip {...args}>
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

export const Close = Template.bind({});

export const Open = Template.bind({});
Open.args = {
  open: true,
};
