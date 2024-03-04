import type { StoryObj, Meta } from "@storybook/react";
import { generateMeta } from "./storybook";
import { WorkspaceProxyPage } from "./WorkspaceProxyPage";

const meta: Meta = {
  title: "pages/Health/WorkspaceProxy",
  ...generateMeta({
    path: "/health/workspace-proxy",
    element: <WorkspaceProxyPage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as WorkspaceProxy };
