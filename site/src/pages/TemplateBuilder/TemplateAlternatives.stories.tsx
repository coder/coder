import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	reactRouterParameters,
	withRouter,
} from "storybook-addon-remix-react-router";
import { TemplateAlternatives } from "./TemplateAlternatives";

const meta: Meta<typeof TemplateAlternatives> = {
	title: "pages/TemplateBuilder/TemplateAlternatives",
	component: TemplateAlternatives,
	decorators: [withRouter],
	parameters: {
		reactRouter: reactRouterParameters({
			routing: { path: "/templates/new/builder" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof TemplateAlternatives>;

export const Default: Story = {};
