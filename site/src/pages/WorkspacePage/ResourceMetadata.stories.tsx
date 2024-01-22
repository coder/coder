import { MockWorkspaceResource } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react";
import { ResourceMetadata } from "./ResourceMetadata";

const meta: Meta<typeof ResourceMetadata> = {
  title: "pages/WorkspacePage/ResourceMetadata",
  component: ResourceMetadata,
};

export default meta;
type Story = StoryObj<typeof ResourceMetadata>;

export const Markdown: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      metadata: [
        { key: "text", value: "hello", sensitive: false },
        { key: "link", value: "[hello](#)", sensitive: false },
        { key: "b/i", value: "_hello_, **friend**!", sensitive: false },
        { key: "coder", value: "`beep boop`", sensitive: false },
      ],
    },
  },
};

export const WithLongStrings: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      metadata: [
        {
          key: "xxxxxxxxxxxx",
          value: "14",
          sensitive: false,
        },
        {
          key: "Long",
          value: "The quick brown fox jumped over the lazy dog",
          sensitive: false,
        },
        {
          key: "Really long",
          value:
            "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
          sensitive: false,
        },
      ],
    },
  },
};
