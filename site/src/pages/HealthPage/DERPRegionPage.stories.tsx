import type { Meta, StoryObj } from "@storybook/react";
import { MockHealth } from "testHelpers/entities";
import { DERPRegionPage } from "./DERPRegionPage";
import { generateMeta } from "./storybook";

const firstRegionId = Object.values(MockHealth.derp.regions)[0]!.region
	?.RegionID;

const meta: Meta = {
	title: "pages/Health/DERPRegion",
	...generateMeta({
		path: "/health/derp/regions/:regionId",
		element: <DERPRegionPage />,
		params: { regionId: firstRegionId?.toString() || "" },
	}),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as DERPRegion };
