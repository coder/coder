import type { Meta, StoryObj } from "@storybook/react-vite";
import { BuildingTemplateLoader } from "./BuildingTemplateLoader";

const meta: Meta<typeof BuildingTemplateLoader> = {
	title: "pages/TemplateBuilder/BuildingTemplateLoader",
	component: BuildingTemplateLoader,
	decorators: [
		(Story) => (
			<div style={{ width: "100%", height: 600 }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof BuildingTemplateLoader>;

export const Default: Story = {};
