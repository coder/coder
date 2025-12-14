import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	FullWidthPageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "./FullWidthPageHeader";

const meta: Meta<typeof FullWidthPageHeader> = {
	title: "components/FullWidthPageHeader",
	component: FullWidthPageHeader,
};

export default meta;
type Story = StoryObj<typeof FullWidthPageHeader>;

export const WithTitle: Story = {
	args: {
		children: (
			<>
				<PageHeaderTitle>Templates</PageHeaderTitle>
			</>
		),
	},
};

export const WithSubtitle: Story = {
	args: {
		children: (
			<>
				<PageHeaderTitle>Templates</PageHeaderTitle>
				<PageHeaderSubtitle>
					Create a new workspace from a Template
				</PageHeaderSubtitle>
			</>
		),
	},
};
