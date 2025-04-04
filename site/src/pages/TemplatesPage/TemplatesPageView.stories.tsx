import type { Meta, StoryObj } from "@storybook/react";
import { getDefaultFilterProps } from "components/Filter/storyHelpers";
import { chromaticWithTablet } from "testHelpers/chromatic";
import {
	MockTemplate,
	MockTemplateExample,
	MockTemplateExample2,
	mockApiError,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { TemplatesPageView } from "./TemplatesPageView";

const meta: Meta<typeof TemplatesPageView> = {
	title: "pages/TemplatesPage",
	decorators: [withDashboardProvider],
	parameters: { chromatic: chromaticWithTablet },
	component: TemplatesPageView,
	args: {
		...getDefaultFilterProps({
			query: "deprecated:false",
			menus: {},
			values: {},
		}),
	},
};

export default meta;
type Story = StoryObj<typeof TemplatesPageView>;

export const WithTemplates: Story = {
	args: {
		canCreateTemplates: true,
		error: undefined,
		templates: [
			MockTemplate,
			{
				...MockTemplate,
				active_user_count: -1,
				description: "🚀 Some new template that has no activity data",
				icon: "/icon/goland.svg",
			},
			{
				...MockTemplate,
				active_user_count: 150,
				description: "😮 Wow, this one has a bunch of usage!",
				icon: "",
			},
			{
				...MockTemplate,
				description:
					"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. ",
			},
			{
				...MockTemplate,
				name: "template-without-icon",
				display_name: "No Icon",
				description: "This one has no icon",
				icon: "",
			},
			{
				...MockTemplate,
				name: "template-without-icon-deprecated",
				display_name: "Deprecated No Icon",
				description: "This one has no icon and is deprecated",
				deprecated: true,
				deprecation_message: "This template is so old, it's deprecated",
				icon: "",
			},
			{
				...MockTemplate,
				name: "deprecated-template",
				display_name: "Deprecated",
				description: "Template is incompatible",
			},
		],
		examples: [],
		workspacePermissions: {
			[MockTemplate.organization_id]: {
				createWorkspaceForUserID: true,
			},
		},
	},
};

export const MultipleOrganizations: Story = {
	args: {
		...WithTemplates.args,
		showOrganizations: true,
	},
};

export const CannotCreateWorkspaces: Story = {
	args: {
		...WithTemplates.args,
		workspacePermissions: {
			[MockTemplate.organization_id]: {
				createWorkspaceForUserID: false,
			},
		},
	},
};

export const WithFilteredAllTemplates: Story = {
	args: {
		...WithTemplates.args,
		templates: [],
		...getDefaultFilterProps({
			query: "deprecated:false searchnotfound",
			menus: {},
			values: {},
			used: true,
		}),
	},
};

export const EmptyCanCreate: Story = {
	args: {
		canCreateTemplates: true,
		error: undefined,
		templates: [],
		examples: [MockTemplateExample, MockTemplateExample2],
	},
};

export const EmptyCannotCreate: Story = {
	args: {
		error: undefined,
		templates: [],
		examples: [MockTemplateExample, MockTemplateExample2],
		canCreateTemplates: false,
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Something went wrong fetching templates.",
		}),
		templates: undefined,
		examples: undefined,
		canCreateTemplates: false,
	},
};

export const WithValidationError: Story = {
	args: {
		error: mockApiError({
			message: "Something went wrong fetching templates.",
			detail:
				"This is a more detailed error message that should help you understand what went wrong.",
			validations: [
				{
					field: "search",
					detail: "That search query was invalid, why did you do that?",
				},
			],
		}),
		templates: undefined,
		examples: undefined,
		canCreateTemplates: false,
	},
};
