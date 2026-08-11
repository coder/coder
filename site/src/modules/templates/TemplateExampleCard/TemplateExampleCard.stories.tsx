import type { Meta, StoryObj } from "@storybook/react-vite";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { chromatic } from "#/testHelpers/chromatic";
import {
	MockDeploymentConfig,
	MockTemplateExample,
	MockTemplateExample2,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { TemplateExampleCard } from "./TemplateExampleCard";

const meta: Meta<typeof TemplateExampleCard> = {
	title: "modules/templates/TemplateExampleCard",
	parameters: {
		chromatic,
		user: MockUserOwner,
		queries: [
			{
				key: deploymentConfigQueryKey,
				data: MockDeploymentConfig,
			},
		],
	},
	decorators: [withAuthProvider],
	component: TemplateExampleCard,
	args: {
		example: MockTemplateExample,
	},
};

export default meta;
type Story = StoryObj<typeof TemplateExampleCard>;

export const Example: Story = {};

export const ByTag: Story = {
	args: {
		activeTag: "cloud",
	},
};

export const LotsOfTags: Story = {
	args: {
		example: {
			...MockTemplateExample2,
			tags: ["omg", "so many tags", "look at all these", "so cool"],
		},
	},
};
