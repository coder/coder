import {
  mockApiError,
  MockOrganization,
  MockTemplateExample,
  MockTemplateExample2,
} from "testHelpers/entities";
import { getTemplatesByTag } from "utils/starterTemplates";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof StarterTemplatesPageView> = {
  title: "pages/StarterTemplatesPageView",
  component: StarterTemplatesPageView,
};

export default meta;
type Story = StoryObj<typeof StarterTemplatesPageView>;

export const Default: Story = {
  args: {
    context: {
      organizationId: MockOrganization.id,
      error: undefined,
      starterTemplatesByTag: getTemplatesByTag([
        MockTemplateExample,
        MockTemplateExample2,
      ]),
    },
  },
};

export const Error: Story = {
  args: {
    context: {
      organizationId: MockOrganization.id,
      error: mockApiError({
        message: "Error on loading the template examples",
      }),
      starterTemplatesByTag: undefined,
    },
  },
};
