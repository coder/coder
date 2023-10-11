import { Meta, StoryObj } from "@storybook/react";
import {
  mockApiError,
  MockDeploymentDAUResponse,
  MockEntitlementsWithUserLimit,
} from "testHelpers/entities";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";

const meta: Meta<typeof GeneralSettingsPageView> = {
  title: "pages/DeploySettingsPage/GeneralSettingsPageView",
  component: GeneralSettingsPageView,
  args: {
    deploymentOptions: [
      {
        name: "Access URL",
        description:
          "The URL that users will use to access the Coder deployment.",
        flag: "access-url",
        flag_shorthand: "",
        value: "https://dev.coder.com",
        hidden: false,
      },
      {
        name: "Wildcard Access URL",
        description:
          'Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".',
        flag: "wildcard-access-url",
        flag_shorthand: "",
        value: "*--apps.dev.coder.com",
        hidden: false,
      },
      {
        name: "Experiments",
        description:
          "Enable one or more experiments. These are not ready for production. Separate multiple experiments with commas, or enter '*' to opt-in to all available experiments.",
        flag: "experiments",
        value: ["*", "moons", "single_tailnet", "deployment_health_page"],
        flag_shorthand: "",
        hidden: false,
      },
    ],
    deploymentDAUs: MockDeploymentDAUResponse,
  },
};

export default meta;
type Story = StoryObj<typeof GeneralSettingsPageView>;

export const Page: Story = {};

export const WithUserLimit: Story = {
  args: {
    deploymentDAUs: MockDeploymentDAUResponse,
    entitlements: MockEntitlementsWithUserLimit,
  },
};

export const NoDAUs: Story = {
  args: {
    deploymentDAUs: undefined,
  },
};

export const DAUError: Story = {
  args: {
    deploymentDAUs: undefined,
    deploymentDAUsError: mockApiError({
      message: "Error fetching DAUs.",
    }),
  },
};
