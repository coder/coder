import type { Meta, StoryObj } from "@storybook/react-vite";
import { generateMeta } from "../storybook";
import DatabasePage from "./DatabasePage";

const meta: Meta = {
	title: "pages/HealthPage/DatabasePage",
	...generateMeta({
		path: "/health/database",
		element: <DatabasePage />,
	}),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as Database };
