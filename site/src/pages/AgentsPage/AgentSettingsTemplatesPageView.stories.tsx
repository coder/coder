import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockTemplate } from "#/testHelpers/entities";
import { AgentSettingsTemplatesPageView } from "./AgentSettingsTemplatesPageView";

const manyTemplates = [
	{ id: "t-01", name: "docker-dev", display_name: "Docker Development" },
	{
		id: "t-02",
		name: "kubernetes-prod",
		display_name: "Kubernetes Production",
	},
	{ id: "t-03", name: "aws-windows", display_name: "AWS Windows Desktop" },
	{ id: "t-04", name: "gcp-linux", display_name: "GCP Linux Workspace" },
	{
		id: "t-05",
		name: "azure-dotnet",
		display_name: "Azure .NET Environment",
	},
	{ id: "t-06", name: "ml-jupyter", display_name: "ML Jupyter Notebook" },
	{
		id: "t-07",
		name: "data-eng-spark",
		display_name: "Data Engineering (Spark)",
	},
	{
		id: "t-08",
		name: "frontend-vite",
		display_name: "Frontend (Vite + React)",
	},
].map((t) => ({ ...MockTemplate, ...t }));

const meta = {
	title: "pages/AgentsPage/AgentSettingsTemplatesPageView",
	component: AgentSettingsTemplatesPageView,
	args: {
		templatesData: manyTemplates,
		allowlistData: { template_ids: [] },
		isLoading: false,
		hasError: false,
		isSaving: false,
		isSaveError: false,
		onRetry: fn(),
		onSaveAllowlist: fn(),
	},
} satisfies Meta<typeof AgentSettingsTemplatesPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsTemplatesPageView>;

export const TemplateAllowlist: Story = {
	play: async ({ canvasElement, step, args }) => {
		const canvas = within(canvasElement);

		await step("starts empty", async () => {
			await canvas.findByText(/no templates selected/i);
			const saveBtn = await canvas.findByRole("button", {
				name: "Save",
			});
			expect(saveBtn).toBeDisabled();
		});

		await step("search filters by display name", async () => {
			const input = canvas.getByPlaceholderText("Select templates...");
			await userEvent.click(input);

			// Type a partial display name.
			await userEvent.type(input, "Docker");

			// The matching template should be visible.
			await waitFor(() => {
				expect(
					canvas.getByRole("option", { name: "Docker Development" }),
				).toBeVisible();
			});

			// A non-matching template should not be visible.
			await waitFor(() => {
				expect(
					canvas.queryByRole("option", { name: "Kubernetes Production" }),
				).not.toBeInTheDocument();
			});

			// Clear the search and verify the full list returns.
			await userEvent.clear(input);
			await waitFor(() => {
				expect(
					canvas.getByRole("option", { name: "Kubernetes Production" }),
				).toBeVisible();
			});

			// Close dropdown by pressing Escape so the next step starts clean.
			await userEvent.keyboard("{Escape}");
		});

		await step("select one template and save", async () => {
			const input = canvas.getByPlaceholderText("Select templates...");
			await userEvent.click(input);
			await userEvent.click(
				await canvas.findByRole("option", {
					name: "Docker Development",
				}),
			);

			await waitFor(() => {
				expect(canvas.getByText("1 template selected")).toBeInTheDocument();
			});

			const saveBtn = canvas.getByRole("button", { name: "Save" });
			expect(saveBtn).toBeEnabled();
			await userEvent.click(saveBtn);

			await waitFor(() => {
				expect(args.onSaveAllowlist).toHaveBeenCalledWith(
					{ template_ids: ["t-01"] },
					expect.anything(),
				);
			});
		});

		await step("add the remaining seven and save", async () => {
			const input = canvas.getByLabelText("Select allowed templates");
			await userEvent.click(input);

			for (const name of [
				"Kubernetes Production",
				"AWS Windows Desktop",
				"GCP Linux Workspace",
				"Azure .NET Environment",
				"ML Jupyter Notebook",
				"Data Engineering (Spark)",
				"Frontend (Vite + React)",
			]) {
				await userEvent.click(await canvas.findByRole("option", { name }));
			}

			await waitFor(() => {
				expect(canvas.getByText("8 templates selected")).toBeInTheDocument();
			});

			const saveBtn = canvas.getByRole("button", { name: "Save" });
			await userEvent.click(saveBtn);

			await waitFor(() => {
				expect(args.onSaveAllowlist).toHaveBeenLastCalledWith(
					{
						template_ids: expect.arrayContaining([
							"t-01",
							"t-02",
							"t-03",
							"t-04",
							"t-05",
							"t-06",
							"t-07",
							"t-08",
						]),
					},
					expect.anything(),
				);
			});
		});
	},
};
