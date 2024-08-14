import type { Meta, StoryObj } from "@storybook/react";
import {
  MockTemplate,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockTemplateVersionParameter4,
} from "testHelpers/entities";
import { TemplateEmbedPageView } from "./TemplateEmbedPage";

const meta: Meta<typeof TemplateEmbedPageView> = {
  title: "pages/TemplatePage/TemplateEmbedPageView",
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
