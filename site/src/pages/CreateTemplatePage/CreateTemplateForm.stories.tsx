import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockTemplate,
  MockTemplateExample,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  MockTemplateVersionVariable4,
  MockTemplateVersionVariable5,
} from "testHelpers/entities";
import { CreateTemplateForm } from "./CreateTemplateForm";

const meta: Meta<typeof CreateTemplateForm> = {
  title: "pages/CreateTemplatePage/CreateTemplateForm",
  component: CreateTemplateForm,
  args: {
    isSubmitting: false,
    onCancel: action("onCancel"),
  },
};

export default meta;
type Story = StoryObj<typeof CreateTemplateForm>;

export const Upload: Story = {
  args: {
    upload: {
      isUploading: false,
      onRemove: () => {},
      onUpload: () => {},
      file: undefined,
    },
  },
};

export const StarterTemplate: Story = {
  args: {
    starterTemplate: MockTemplateExample,
  },
};

export const DuplicateTemplateWithVariables: Story = {
  args: {
    copiedTemplate: MockTemplate,
    variables: [
      MockTemplateVersionVariable1,
      MockTemplateVersionVariable2,
      MockTemplateVersionVariable3,
      MockTemplateVersionVariable4,
      MockTemplateVersionVariable5,
    ],
  },
};
