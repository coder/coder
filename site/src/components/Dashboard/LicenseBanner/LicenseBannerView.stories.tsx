import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { LicenseBannerView } from "./LicenseBannerView";

const meta: Meta<typeof LicenseBannerView> = {
  title: "components/LicenseBannerView",
  parameters: { chromatic },
  component: LicenseBannerView,
};

export default meta;
type Story = StoryObj<typeof LicenseBannerView>;

export const OneWarning: Story = {
  args: {
    errors: [],
    warnings: ["You have exceeded the number of seats in your license."],
  },
};

export const TwoWarnings: Story = {
  args: {
    errors: [],
    warnings: [
      "You have exceeded the number of seats in your license.",
      "You are flying too close to the sun.",
    ],
  },
};

export const OneError: Story = {
  args: {
    errors: [
      "You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.",
    ],
    warnings: [],
  },
};
