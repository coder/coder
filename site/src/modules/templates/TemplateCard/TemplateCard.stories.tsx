import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
  MockTemplate,
} from "testHelpers/entities";
import { TemplateCard } from "./TemplateCard";

const meta: Meta<typeof TemplateCard> = {
  title: "modules/templates/TemplateCard",
  parameters: { chromatic },
  component: TemplateCard,
  args: {
    template: MockTemplate,
  },
};

export default meta;
type Story = StoryObj<typeof TemplateCard>;

export const Template: Story = {};

export const DeprecatedTemplate: Story = {  args: {
  template: {
    ...MockTemplate,
    deprecated: true
  }
},};

export const LongContentTemplate: Story = {
  args: {
    template: {
      ...MockTemplate,
      display_name: 'Very Long Template Name',
      organization_name: 'Very Long Organization Name',
      description: 'This is a very long test description. This is a very long test description. This is a very long test description. This is a very long test description',
      active_user_count: 999
    }
  },
};
