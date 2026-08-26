import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { getDefaultFilterProps } from "#/components/Filter/storyHelpers";
import type { TemplateFilterState } from "#/pages/TemplatesPage/TemplatesFilter";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { TemplatesPageView } from "./TemplatesPageView";

const templates = [
	{
		id: "t-01",
		name: "docker-containers",
		display_name: "Docker containers",
		updated_at: "2026-06-23T12:00:00.000Z",
		active_user_count: 125,
		agents_allowed: true,
	},
	{
		id: "t-02",
		name: "product-ops-engineering",
		display_name: "Product ops engineering",
		updated_at: "2026-06-20T12:00:00.000Z",
		active_user_count: 12,
		agents_allowed: false,
	},
	{
		id: "t-03",
		name: "ai-webinar",
		display_name: "AI webinar",
		updated_at: "2026-06-04T12:00:00.000Z",
		active_user_count: 3,
		agents_allowed: true,
	},
	{
		id: "t-04",
		name: "fast-workspace",
		display_name: "A fast workspace",
		updated_at: "2026-05-23T12:00:00.000Z",
		active_user_count: 1,
		agents_allowed: false,
	},
].map((template): TypesGen.Template => ({
	...MockTemplate,
	...template,
}));

const filterState = getDefaultFilterProps<TemplateFilterState>({
	menus: {},
	values: {},
});

const meta = {
	title: "pages/AISettingsPage/TemplatesPage/TemplatesPageView",
	component: TemplatesPageView,
	decorators: [withDashboardProvider],
	args: {
		filterState,
		templates,
		isLoading: false,
		error: undefined,
		pendingTemplateIDs: new Set<string>(),
		onRetry: fn(),
		onToggleAgentsAllowed: fn(),
	},
} satisfies Meta<typeof TemplatesPageView>;

export default meta;
type Story = StoryObj<typeof TemplatesPageView>;

export const MixedToggles: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Docker containers")).toBeVisible();
		expect(
			canvas.getAllByText(MockTemplate.organization_display_name)[0],
		).toBeVisible();
		expect(canvas.getByText("125 developers")).toBeVisible();
		const table = canvas.getByRole("table", {
			name: "Templates Coder Agents can use to create workspaces",
		});
		expect(
			within(table).getByRole("columnheader", {
				name: "Coder Agents workspace creation",
			}),
		).toBeInTheDocument();
		const rows = within(table).getAllByRole("row");
		expect(within(rows[1]).getByText("Docker containers")).toBeVisible();
		expect(within(rows[2]).getByText("Product ops engineering")).toBeVisible();
		expect(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to create workspaces using Docker containers in My Organization",
			}),
		).toBeChecked();
		expect(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to create workspaces using Product ops engineering in My Organization",
			}),
		).not.toBeChecked();
	},
};

export const ToggleTemplate: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to create workspaces using Docker containers in My Organization",
			}),
		);
		expect(args.onToggleAgentsAllowed).toHaveBeenCalledWith(
			templates[0],
			false,
		);
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		templates: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByRole("status")).toBeVisible();
	},
};

export const LoadError: Story = {
	args: {
		error: new Error("Templates request failed"),
		templates: undefined,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Failed to load templates.")).toBeVisible();
		expect(canvas.queryByRole("table")).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onRetry).toHaveBeenCalled();
	},
};

export const RefetchError: Story = {
	args: {
		error: new Error("Templates request failed"),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Failed to load templates.")).toBeVisible();
		expect(
			canvas.getByRole("table", {
				name: "Templates Coder Agents can use to create workspaces",
			}),
		).toBeVisible();
		expect(canvas.getByText("Docker containers")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onRetry).toHaveBeenCalled();
	},
};

export const ValidationError: Story = {
	args: {
		error: mockApiError({
			message: "Invalid template search query.",
			validations: [
				{
					field: "search",
					detail: "The template filter is invalid.",
				},
			],
		}),
		templates: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("The template filter is invalid."),
		).toBeVisible();
		expect(canvas.queryByRole("table")).not.toBeInTheDocument();
		expect(canvas.queryByRole("status")).not.toBeInTheDocument();
	},
};

export const Empty: Story = {
	args: {
		templates: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("No templates found.")).toBeVisible();
		expect(
			canvas.getByText(
				"Create a template before configuring whether Coder Agents can create workspaces.",
			),
		).toBeVisible();
	},
};

export const FilteredEmpty: Story = {
	args: {
		templates: [],
		filterState: {
			...filterState,
			filter: {
				...filterState.filter,
				query: "missing",
				used: true,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("No results matched your search."),
		).toBeVisible();
	},
};

export const MixedOrganizations: Story = {
	args: {
		templates: [
			{
				...templates[0],
				organization_id: "engineering-id",
				organization_name: "engineering",
				organization_display_name: "Engineering",
			},
			{
				...templates[0],
				id: "product-template-id",
				organization_id: "product-id",
				organization_name: "product",
				organization_display_name: "Product",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const engineeringSwitch = await canvas.findByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Docker containers in Engineering",
		});
		const productSwitch = canvas.getByRole("switch", {
			name: "Allow Coder Agents to create workspaces using Docker containers in Product",
		});
		expect(engineeringSwitch).toBeChecked();
		expect(productSwitch).toBeChecked();
	},
};

export const UpdatingOneTemplate: Story = {
	args: {
		pendingTemplateIDs: new Set([templates[1].id]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByRole("switch", {
				name: "Allow Coder Agents to create workspaces using Product ops engineering in My Organization",
			}),
		).toBeDisabled();
		expect(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to create workspaces using Docker containers in My Organization",
			}),
		).toBeEnabled();
	},
};
