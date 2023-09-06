import { Meta, StoryObj } from "@storybook/react";
import { HealthPageView } from "./HealthPage";
import { MockHealth } from "testHelpers/entities";

const meta: Meta<typeof HealthPageView> = {
  title: "pages/HealthPageView",
  component: HealthPageView,
  args: {
    tab: {
      value: "derp",
      set: () => {},
    },
    healthStatus: MockHealth,
  },
};

export default meta;
type Story = StoryObj<typeof HealthPageView>;

export const HealthPage: Story = {};

export const UnhealthPage: Story = {
  args: {
    healthStatus: {
      ...MockHealth,
      healthy: false,
      derp: {
        ...MockHealth.derp,
        healthy: false,
      },
    },
  },
};
