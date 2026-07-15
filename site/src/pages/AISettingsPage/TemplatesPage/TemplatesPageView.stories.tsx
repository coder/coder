import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockTemplate } from "#/testHelpers/entities";
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
].map(
	(template): TypesGen.Template => ({
		...MockTemplate,
		...template,
	}),
);

const meta = {
	title: "pages/AISettingsPage/TemplatesPage/TemplatesPageView",
	component: TemplatesPageView,
	// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: {
		templates,
		isLoading: false,
		error: undefined,
		pendingTemplateIDs: new Set<string>(),
		updateErrors: new Map<string, unknown>(),
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
		expect(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to use Docker containers",
			}),
		).toBeChecked();
		expect(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to use Product ops engineering",
			}),
		).not.toBeChecked();
	},
};

export const ToggleTemplate: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to use Docker containers",
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
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Failed to load templates.")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onRetry).toHaveBeenCalled();
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
				"Create a template before configuring Coder Agents access.",
			),
		).toBeVisible();
	},
};

export const MixedOrganizations: Story = {
	args: {
		templates: [
			templates[0],
			{
				...templates[1],
				organization_id: "engineering-id",
				organization_name: "engineering",
				organization_display_name: "Engineering",
			},
			{
				...templates[2],
				organization_id: "product-id",
				organization_name: "product",
				organization_display_name: "Product",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Engineering")).toBeVisible();
		expect(canvas.getByText("Product")).toBeVisible();
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
				name: "Allow Coder Agents to use Product ops engineering",
			}),
		).toBeDisabled();
		expect(
			canvas.getByRole("switch", {
				name: "Allow Coder Agents to use Docker containers",
			}),
		).toBeEnabled();
	},
};

export const MutationError: Story = {
	args: {
		updateErrors: new Map<string, unknown>([
			["t-01", "Template access is locked."],
			["t-03", "Something went wrong."],
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const alerts = await canvas.findAllByRole("alert");
		expect(alerts).toHaveLength(2);
		expect(alerts[0]).toHaveTextContent(
			"Docker containers: Template access is locked.",
		);
		expect(alerts[1]).toHaveTextContent("AI webinar: Something went wrong.");
		expect(canvas.getByText("Docker containers")).toBeVisible();
	},
};
