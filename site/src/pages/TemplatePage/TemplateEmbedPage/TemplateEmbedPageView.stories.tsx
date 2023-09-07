import type { Meta, StoryObj } from "@storybook/react";

import { TemplateEmbedPageView } from "./TemplateEmbedPage";
import {
  MockTemplate,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockTemplateVersionParameter4,
} from "testHelpers/entities";

const meta: Meta<typeof TemplateEmbedPageView> = {
  title: "pages/TemplateEmbedPageView",
  component: TemplateEmbedPageView,
  args: {
    template: MockTemplate,
  },
};

export default meta;
type Story = StoryObj<typeof TemplateEmbedPageView>;

export const NoParameters: Story = {
  args: {
    templateParameters: [],
  },
};

export const WithParameters: Story = {
  args: {
    templateParameters: [
      MockTemplateVersionParameter1,
      MockTemplateVersionParameter2,
      MockTemplateVersionParameter3,
      MockTemplateVersionParameter4,
    ],
  },
};
