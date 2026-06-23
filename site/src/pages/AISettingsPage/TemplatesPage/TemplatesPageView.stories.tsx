import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockTemplate } from "#/testHelpers/entities";
import { TemplatesPageView } from "./TemplatesPageView";

const templateIDs = ["t-01", "t-02", "t-03", "t-04", "t-05", "t-06"];

const templates: TypesGen.Template[] = [
	{
		id: templateIDs[0],
		name: "docker-containers",
		display_name: "Docker containers",
		description: "Develop inside Docker containers.",
		icon: "/icon/docker.png",
		updated_at: "2026-06-23T12:00:00.000Z",
		active_user_count: 125,
	},
	{
		id: templateIDs[1],
		name: "product-ops-engineering",
		display_name: "Product ops engineering",
		description: "Workspace for product operations engineering.",
		updated_at: "2026-06-20T12:00:00.000Z",
		active_user_count: 12,
	},
	{
		id: templateIDs[2],
		name: "ai-webinar",
		display_name: "AI webinar",
		description: "Workspace for webinar demos.",
		updated_at: "2026-06-04T12:00:00.000Z",
		active_user_count: 3,
	},
	{
		id: templateIDs[3],
		name: "fast-workspace",
		display_name: "A fast workspace",
		description: "A minimal workspace that starts quickly.",
		updated_at: "2026-05-23T12:00:00.000Z",
		active_user_count: 1,
	},
	{
		id: templateIDs[4],
		name: "aws-ec2",
		display_name: "AWS EC2",
		description: "Provision AWS EC2 instances as workspaces.",
		updated_at: "2026-01-23T12:00:00.000Z",
		active_user_count: 0,
	},
	{
		id: templateIDs[5],
		name: "gke-sandbox",
		display_name: "gke-sandbox",
		description: "Sandbox workspace on GKE.",
		updated_at: "2025-06-23T12:00:00.000Z",
		active_user_count: 0,
	},
].map((template) => ({ ...MockTemplate, ...template }));

const meta = {
	title: "pages/AISettingsPage/TemplatesPage/TemplatesPageView",
	component: TemplatesPageView,
	args: {
		templatesData: templates,
		allowlistData: { template_ids: [templateIDs[0], templateIDs[1]] },
		isLoading: false,
		templatesError: undefined,
		allowlistError: undefined,
		isSaving: false,
		isSaveError: false,
		onRetry: fn(),
		onSaveAllowlist: fn(),
	},
} satisfies Meta<typeof TemplatesPageView>;

export default meta;
type Story = StoryObj<typeof TemplatesPageView>;

export const NoRestrictions: Story = {
	args: {
		allowlistData: { template_ids: [] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("No restrictions set.")).toBeVisible();
		expect(
			canvas.getByText(
				"All templates are available. Add a template to create an allowlist.",
			),
		).toBeVisible();
	},
};

export const TemplateAllowlist: Story = {
	play: async ({ canvasElement, step, args }) => {
		const canvas = within(canvasElement);

		await step("renders allowlisted templates", async () => {
			expect(await canvas.findByText("Docker containers")).toBeVisible();
			expect(canvas.getByText("Product ops engineering")).toBeVisible();
			expect(canvas.getByText("125 developers")).toBeVisible();
			expect(canvas.getByText("12 developers")).toBeVisible();
		});

		await step("removes an allowlisted template", async () => {
			const body = within(document.body);
			await userEvent.click(
				canvas.getByRole("button", { name: "Actions for Docker containers" }),
			);
			await userEvent.click(
				await body.findByRole("menuitem", { name: /remove/i }),
			);
			await waitFor(() => {
				expect(args.onSaveAllowlist).toHaveBeenCalledWith({
					template_ids: [templateIDs[1]],
				});
			});
		});

		await step("searches and adds an available template", async () => {
			const body = within(document.body);
			await userEvent.click(
				canvas.getByRole("button", { name: /add template/i, hidden: true }),
			);
			const searchInput = await body.findByLabelText("Search templates");
			await userEvent.click(searchInput);
			await userEvent.keyboard("webinar");
			expect(searchInput).toHaveValue("webinar");

			expect(
				await body.findByRole("button", { name: /AI webinar/i }),
			).toBeVisible();
			expect(
				body.queryByRole("button", { name: /AWS EC2/i }),
			).not.toBeInTheDocument();

			await userEvent.click(body.getByRole("button", { name: /AI webinar/i }));
			await waitFor(() => {
				expect(args.onSaveAllowlist).toHaveBeenLastCalledWith({
					template_ids: [templateIDs[0], templateIDs[1], templateIDs[2]],
				});
			});
		});
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		templatesData: undefined,
		allowlistData: undefined,
	},
};

export const TemplatesLoadError: Story = {
	args: {
		templatesError: new Error("Templates request failed"),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Failed to load templates.")).toBeVisible();
		expect(
			canvas.getByText("Please check the developer console for more details."),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onRetry).toHaveBeenCalled();
	},
};

export const AllowlistLoadError: Story = {
	args: {
		allowlistError: new Error("Allowlist request failed"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText(
				"Failed to load template allowlist configuration.",
			),
		).toBeVisible();
		expect(canvas.getByRole("button", { name: "Retry" })).toBeVisible();
	},
};

export const SaveError: Story = {
	args: {
		isSaveError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Failed to save template allowlist."),
		).toBeVisible();
	},
};
