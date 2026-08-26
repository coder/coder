import type { Meta, StoryObj } from "@storybook/react-vite";
import { generateMeta } from "../storybook";
import DERPPage from "./DERPPage";

const meta: Meta = {
	title: "pages/HealthPage/DERPPage",
	...generateMeta({
		path: "/health/derp",
		element: <DERPPage />,
	}),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as DERP };
