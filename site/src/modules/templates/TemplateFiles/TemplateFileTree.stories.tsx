import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { TemplateFileTree } from "./TemplateFileTree";
import { FileTree } from "utils/filetree";
import { useTheme } from "@emotion/react";

const fileTree: FileTree = {
  "main.tf": "resource aws_instance my_instance {}",
  "variables.tf": "variable my_var {}",
  "outputs.tf": "output my_output {}",
  folder: {
    "nested.tf": "resource aws_instance my_instance {}",
  },
};

const meta: Meta<typeof TemplateFileTree> = {
  title: "modules/templates/TemplateFileTree",
  parameters: { chromatic },
  component: TemplateFileTree,
  args: {
    fileTree,
    activePath: "main.tf",
  },
  decorators: [
    (Story) => {
      const theme = useTheme();
      return (
        <div
          css={{
            maxWidth: 260,
            borderRadius: 8,
            border: `1px solid ${theme.palette.divider}`,
          }}
        >
          <Story />
        </div>
      );
    },
  ],
};

export default meta;
type Story = StoryObj<typeof TemplateFileTree>;

export const Example: Story = {};

export const NestedOpen: Story = {
  args: {
    activePath: "folder/nested.tf",
  },
};

export const GroupEmptyFolders: Story = {
  args: {
    activePath: "folder/other-folder/another/nested.tf",
    fileTree: {
      "main.tf": "resource aws_instance my_instance {}",
      "variables.tf": "variable my_var {}",
      "outputs.tf": "output my_output {}",
      folder: {
        "other-folder": {
          another: {
            "nested.tf": "resource aws_instance my_instance {}",
          },
        },
      },
    },
  },
};

export const GreyOutHiddenFiles: Story = {
  args: {
    fileTree: {
      ".vite": {
        "config.json": "resource aws_instance my_instance {}",
      },
      ".nextjs": {
        "nested.tf": "resource aws_instance my_instance {}",
      },
      ".terraform.lock.hcl": "{}",
      "main.tf": "resource aws_instance my_instance {}",
      "variables.tf": "variable my_var {}",
      "outputs.tf": "output my_output {}",
    },
  },
};
