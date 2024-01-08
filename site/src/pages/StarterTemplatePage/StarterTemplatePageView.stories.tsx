import { chromatic } from "testHelpers/chromatic";
import { mockApiError, MockTemplateExample } from "testHelpers/entities";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof StarterTemplatePageView> = {
  title: "pages/StarterTemplatePage",
  parameters: { chromatic },
  component: StarterTemplatePageView,
};

export default meta;
type Story = StoryObj<typeof StarterTemplatePageView>;

export const Example: Story = {
  args: {
    error: undefined,
    starterTemplate: MockTemplateExample,
  },
};
export const Error: Story = {
  args: {
    error: mockApiError({
      message: `Example ${MockTemplateExample.id} not found.`,
    }),
    starterTemplate: undefined,
  },
};
