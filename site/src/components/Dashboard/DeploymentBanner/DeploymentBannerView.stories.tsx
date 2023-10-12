import type { Meta, StoryObj } from "@storybook/react";
import { MockDeploymentStats } from "testHelpers/entities";
import { DeploymentBannerView } from "./DeploymentBannerView";

const meta: Meta<typeof DeploymentBannerView> = {
  title: "components/DeploymentBannerView",
  component: DeploymentBannerView,
  args: {
    stats: MockDeploymentStats,
  },
};

export default meta;
type Story = StoryObj<typeof DeploymentBannerView>;

export const Example: Story = {};

export const WithHealthIssues: Story = {
  args: {
    health: {
      healthy: false,
      time: "2023-10-12T23:15:00.000000000Z",
      coder_version: "v2.3.0-devel+8cca4915a",
      access_url: { healthy: false },
      database: { healthy: false },
      derp: { healthy: false },
      websocket: { healthy: false },
    },
  },
};
