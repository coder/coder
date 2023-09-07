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

export const Preview: Story = {};
