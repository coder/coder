import { StoryObj, Meta } from "@storybook/react";
import { DERPRegionPage } from "./DERPRegionPage";
import { MockHealth } from "testHelpers/entities";
import { generateMeta } from "./storybook";

const firstRegionId = Object.values(MockHealth.derp.regions)[0].region
  ?.RegionID;

const meta: Meta = {
  title: "pages/Health/DERPRegion",
  ...generateMeta({
    path: "/health/derp/regions/:regionId",
    element: <DERPRegionPage />,
    params: { regionId: firstRegionId },
  }),
};

export default meta;
type Story = StoryObj;

export const Default: Story = {};
