import type { Meta, StoryObj } from "@storybook/react";
import { MockTemplate, MockTemplateVersion } from "testHelpers/entities";
import { TemplateStats } from "./TemplateStats";

const meta: Meta<typeof TemplateStats> = {
	title: "pages/TemplatePage/TemplateStats",
	component: TemplateStats,
};

export default meta;
type Story = StoryObj<typeof TemplateStats>;

export const Example: Story = {
	args: {
		template: MockTemplate,
		activeVersion: MockTemplateVersion,
	},
};

export const UsedByMany: Story = {
	args: {
		template: {
			...MockTemplate,
			active_user_count: 15,
		},
		activeVersion: MockTemplateVersion,
	},
};

export const ActiveUsersNotLoaded: Story = {
	args: {
		template: {
			...MockTemplate,
			active_user_count: -1,
		},
		activeVersion: MockTemplateVersion,
	},
};

export const LongTemplateVersion: Story = {
	args: {
		template: MockTemplate,
		activeVersion: {
			...MockTemplateVersion,
			name: "thisisareallyreallylongnamefortesting",
		},
	},
	parameters: {
		chromatic: { viewports: [960] },
	},
};

export const SmallViewport: Story = {
	args: {
		template: MockTemplate,
		activeVersion: MockTemplateVersion,
	},
	parameters: {
		chromatic: { viewports: [600] },
	},
};
