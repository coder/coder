import { StoryObj, Meta } from "@storybook/react";
import { WorkspaceProxyPage } from "./WorkspaceProxyPage";
import { HealthLayout } from "./HealthLayout";
import {
  reactRouterOutlet,
  reactRouterParameters,
} from "storybook-addon-react-router-v6";
import { useQueryClient } from "react-query";
import { MockHealth } from "testHelpers/entities";

const meta: Meta = {
  title: "pages/Health/WorkspaceProxy",
  render: HealthLayout,
  parameters: {
    layout: "fullscreen",
    reactRouter: reactRouterParameters({
      routing: reactRouterOutlet(
        { path: "/health/workspace-proxy" },
        <WorkspaceProxyPage />,
      ),
    }),
  },
  decorators: [
    (Story) => {
      const queryClient = useQueryClient();
      queryClient.setQueryData(["health"], MockHealth);
      return <Story />;
    },
  ],
};

export default meta;
type Story = StoryObj;

export const Default: Story = {};
