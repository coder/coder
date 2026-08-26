import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockHealth } from "#/testHelpers/entities";
import { generateMeta } from "../storybook";
import DERPRegionPage from "./DERPRegionPage";

const firstRegionId = Object.values(MockHealth.derp.regions)[0]!.region
	?.RegionID;

const meta: Meta = {
	title: "pages/HealthPage/DERPRegionPage",
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
