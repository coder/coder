import type { Meta, StoryObj } from "@storybook/react-vite";
import { generateMeta } from "../storybook";
import ProvisionerDaemonsPage from "./ProvisionerDaemonsPage";

const meta: Meta = {
	title: "pages/HealthPage/ProvisionerDaemonsPage",
	...generateMeta({
		path: "/health/provisioner-daemons",
		element: <ProvisionerDaemonsPage />,
	}),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as ProvisionerDaemons };
