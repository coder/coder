import { StoryObj, Meta } from "@storybook/react";
import { DERPRegionPage } from "./DERPRegionPage";
import { HealthLayout } from "./HealthLayout";
import {
  reactRouterOutlet,
  reactRouterParameters,
} from "storybook-addon-react-router-v6";
import { useQueryClient } from "react-query";
import { MockHealth } from "testHelpers/entities";

const firstRegionId = Object.values(MockHealth.derp.regions)[0].region
  ?.RegionID;

const meta: Meta = {
  title: "pages/Health/DERPRegion",
  render: HealthLayout,
  parameters: {
    layout: "fullscreen",
    reactRouter: reactRouterParameters({
      location: { pathParams: { regionId: firstRegionId } },
      routing: reactRouterOutlet(
        { path: `/health/derp/regions/:regionId` },
        <DERPRegionPage />,
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
