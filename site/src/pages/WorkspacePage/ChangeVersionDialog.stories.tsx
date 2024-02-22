import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionWithMarkdownMessage,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react";
import { ChangeVersionDialog } from "./ChangeVersionDialog";

const noMessage = {
  ...MockTemplateVersion,
  message: "",
};

const meta: Meta<typeof ChangeVersionDialog> = {
  title: "pages/WorkspacePage/ChangeVersionDialog",
  component: ChangeVersionDialog,
  args: {
    open: true,
    template: MockTemplate,
    templateVersions: [
      MockTemplateVersion,
      MockTemplateVersionWithMarkdownMessage,
      noMessage,
    ],
  },
};

export default meta;
type Story = StoryObj<typeof ChangeVersionDialog>;

export const NoVersionSelected: Story = {};

export const NoMessage: Story = {
  args: {
    defaultTemplateVersion: noMessage,
  },
};

export const ShortMessage: Story = {
  args: {
    defaultTemplateVersion: MockTemplateVersion,
  },
};

export const LongMessage: Story = {
  args: {
    defaultTemplateVersion: MockTemplateVersionWithMarkdownMessage,
  },
};
