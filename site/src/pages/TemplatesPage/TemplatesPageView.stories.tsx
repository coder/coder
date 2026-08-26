import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	getDefaultFilterProps,
	MockMenu,
} from "#/components/Filter/storyHelpers";
import {
	MockTemplate,
	MockTemplateExample,
	MockTemplateExample2,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { pixelWithTablet } from "#/testHelpers/pixel";
import { withDashboardProvider } from "#/testHelpers/storybook";
import type { TemplateFilterState } from "./TemplatesFilter";
import { TemplatesPageView } from "./TemplatesPageView";

const defaultFilterProps = getDefaultFilterProps<TemplateFilterState>({
	menus: {
		organizations: MockMenu,
	},
	values: {
		author: MockUserOwner.username,
	},
});

const meta: Meta<typeof TemplatesPageView> = {
	title: "pages/TemplatesPage",
	decorators: [withDashboardProvider],
	parameters: { pixel: { matrix: pixelWithTablet } },
	component: TemplatesPageView,
	args: {
		filterState: defaultFilterProps,
		templateBuilderEnabled: false,
		templateUpdatePermissions: {},
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
			{
				...MockTemplate,
				name: "deleted-template",
				display_name: "Deleted",
				description: "Template has been deleted",
				deleted: true,
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

export const WithTemplatesBuilderEnabled: Story = {
	args: {
		...WithTemplates.args,
		templateBuilderEnabled: true,
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
		filterState: {
			filter: {
				...defaultFilterProps.filter,
				query: "searchnotfound",
				values: {},
				used: true,
			},
			menus: defaultFilterProps.menus,
		},
	},
};

export const WithUserDropdown: Story = {
	args: {
		...WithTemplates.args,
		filterState: {
			...defaultFilterProps,
			menus: {
				user: MockMenu,
			},
			filter: {
				...defaultFilterProps.filter,
				query: "author:me",
				values: { author: "me" },
			},
		},
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

export const EmptyCanCreateWithBuilder: Story = {
	args: {
		canCreateTemplates: true,
		templateBuilderEnabled: true,
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

const classicParameterFlowTemplates = [
	{
		...MockTemplate,
		id: "template-classic-1",
		name: "classic-one",
		display_name: "Classic One",
		use_classic_parameter_flow: true,
	},
	{
		...MockTemplate,
		id: "template-classic-2",
		name: "classic-two",
		display_name: "Classic Two",
		use_classic_parameter_flow: true,
	},
	{
		...MockTemplate,
		id: "template-classic-without-permission",
		organization_id: "other-organization",
		name: "classic-without-permission",
		display_name: "Classic Without Permission",
		use_classic_parameter_flow: true,
	},
	{
		...MockTemplate,
		id: "template-dynamic",
		name: "dynamic-one",
		display_name: "Dynamic One",
		use_classic_parameter_flow: false,
	},
];

export const ClassicParameterFlowWarning: Story = {
	args: {
		...WithTemplates.args,
		canCreateTemplates: true,
		templates: classicParameterFlowTemplates,
		templateUpdatePermissions: {
			[MockTemplate.organization_id]: true,
			"other-organization": false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const alert = canvas.getByRole("alert");
		expect(
			within(alert).getByText(
				"2 templates still use the classic parameter flow",
			),
		).toBeVisible();
		expect(
			within(alert)
				.getByRole("link", { name: /dynamic parameters docs/i })
				.getAttribute("href"),
		).toContain("/admin/templates/extending-templates/dynamic-parameters");

		const classicCell = canvas.getByRole("cell", { name: /Classic One/ });
		expect(within(classicCell).getByText("Classic parameters")).toBeVisible();

		const inaccessibleCell = canvas.getByRole("cell", {
			name: /Classic Without Permission/,
		});
		expect(
			within(inaccessibleCell).queryByText("Classic parameters"),
		).not.toBeInTheDocument();

		const dynamicCell = canvas.getByRole("cell", { name: /Dynamic One/ });
		expect(
			within(dynamicCell).queryByText("Classic parameters"),
		).not.toBeInTheDocument();
	},
};

export const SingleClassicParameterFlowWarning: Story = {
	args: {
		...WithTemplates.args,
		canCreateTemplates: true,
		templates: [
			classicParameterFlowTemplates[0],
			classicParameterFlowTemplates[3],
		],
		templateUpdatePermissions: {
			[MockTemplate.organization_id]: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			within(canvas.getByRole("alert")).getByText(
				"1 template still uses the classic parameter flow",
			),
		).toBeVisible();
	},
};

export const ClassicParameterFlowWarningHiddenWithoutPermission: Story = {
	args: {
		...ClassicParameterFlowWarning.args,
		canCreateTemplates: true,
		templateUpdatePermissions: {
			[MockTemplate.organization_id]: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
		expect(canvas.queryByText("Classic parameters")).not.toBeInTheDocument();
	},
};

export const ClassicParameterFlowWarningWithoutCreatePermission: Story = {
	args: {
		...ClassicParameterFlowWarning.args,
		canCreateTemplates: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.getByRole("alert")).toBeVisible();
		expect(canvas.getAllByText("Classic parameters")).toHaveLength(2);
	},
};

export const WithoutClassicParameterFlowTemplates: Story = {
	args: {
		...WithTemplates.args,
		canCreateTemplates: true,
		templates: [classicParameterFlowTemplates[3]],
		templateUpdatePermissions: {
			[MockTemplate.organization_id]: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
		expect(canvas.queryByText("Classic parameters")).not.toBeInTheDocument();
	},
};
