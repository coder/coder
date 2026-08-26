import type { Meta, StoryObj } from "@storybook/react-vite";
import { TemplateAlternatives } from "./TemplateAlternatives";

const meta: Meta<typeof TemplateAlternatives> = {
	title:
		"pages/TemplatesPage/CreateTemplatePage/TemplateBuilderPage/TemplateAlternatives",
	component: TemplateAlternatives,
};

export default meta;
type Story = StoryObj<typeof TemplateAlternatives>;

export const Default: Story = {};
