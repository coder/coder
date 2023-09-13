import {
  mockApiError,
  MockOrganization,
  MockTemplateExample,
} from "testHelpers/entities";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof StarterTemplatePageView> = {
  title: "pages/StarterTemplatePageView",
  component: StarterTemplatePageView,
};

export default meta;
type Story = StoryObj<typeof StarterTemplatePageView>;

export const Default: Story = {
  args: {
    context: {
      exampleId: MockTemplateExample.id,
      organizationId: MockOrganization.id,
      error: undefined,
      starterTemplate: MockTemplateExample,
    },
  },
};
export const Error: Story = {
  args: {
    context: {
      exampleId: MockTemplateExample.id,
      organizationId: MockOrganization.id,
      error: mockApiError({
        message: `Example ${MockTemplateExample.id} not found.`,
      }),
      starterTemplate: undefined,
    },
  },
};
