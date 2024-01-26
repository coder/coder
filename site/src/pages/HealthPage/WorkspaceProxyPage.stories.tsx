import { StoryObj, Meta } from "@storybook/react";
import { WorkspaceProxyPage } from "./WorkspaceProxyPage";
import { generateMeta } from "./storybook";

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
