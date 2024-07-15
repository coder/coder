import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
  mockApiError,
  MockTemplateExample,
  MockTemplateExample2,
} from "testHelpers/entities";
import { getTemplatesByTag } from "utils/templateAggregators";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";

const meta: Meta<typeof StarterTemplatesPageView> = {
  title: "pages/StarterTemplatesPage",
  parameters: { chromatic },
  component: StarterTemplatesPageView,
};

export default meta;
type Story = StoryObj<typeof StarterTemplatesPageView>;

export const Example: Story = {
  args: {
    error: undefined,
    starterTemplatesByTag: getTemplatesByTag([
      MockTemplateExample,
      MockTemplateExample2,
    ]),
  },
};

export const Error: Story = {
  args: {
    error: mockApiError({
      message: "Error on loading the template examples",
    }),
    starterTemplatesByTag: undefined,
  },
};
