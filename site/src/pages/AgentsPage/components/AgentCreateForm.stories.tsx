import { MockWorkspace } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { AgentCreateForm } from "./AgentCreateForm";

const modelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const meta: Meta<typeof AgentCreateForm> = {
	title: "pages/AgentsPage/AgentCreateForm",
	component: AgentCreateForm,
	decorators: [withDashboardProvider],
	args: {
		onCreateChat: fn(),
		isCreating: false,
		createError: undefined,
		modelCatalog: null,
		modelOptions: [...modelOptions],
		isModelCatalogLoading: false,
		modelConfigs: [],
		isModelConfigsLoading: false,
		modelCatalogError: undefined,
	},
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [],
			count: 0,
		});
	},
};

export default meta;
type Story = StoryObj<typeof AgentCreateForm>;

const defaultArgs = meta.args;

export const Default: Story = {};

const mockWorkspaces = [
	{
		...MockWorkspace,
		id: "ws-1",
		name: "my-project",
		owner_name: "johndoe",
		owner_id: "user-1",
	},
	{
		...MockWorkspace,
		id: "ws-2",
		name: "my-project",
		owner_name: "janedoe",
		owner_id: "user-2",
	},
	{
		...MockWorkspace,
		id: "ws-3",
		name: "backend-api",
		owner_name: "johndoe",
		owner_id: "user-1",
	},
];

export const WithWorkspaces: Story = {
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: mockWorkspaces,
			count: mockWorkspaces.length,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			const trigger = canvas.getByText("Workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(canvas.getByText("Workspace").closest("button")!);
		// Wait for the portalled combobox dropdown to appear so Chromatic
		// captures it.
		await within(canvasElement.ownerDocument.body).findByRole("dialog");
	},
};

export const SearchWorkspaces: Story = {
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: mockWorkspaces,
			count: mockWorkspaces.length,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			const trigger = canvas.getByText("Workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(canvas.getByText("Workspace").closest("button")!);

		const body = within(canvasElement.ownerDocument.body);
		await body.findByRole("dialog");

		// Type in the search input to filter workspaces.
		const searchInput = body.getByPlaceholderText("Search workspaces...");
		await userEvent.type(searchInput, "backend");

		// Only the matching workspace should remain visible.
		await waitFor(() => {
			const options = body.getAllByRole("option");
			// "Auto-create Workspace" is filtered out, only
			// "johndoe/backend-api" matches.
			expect(options).toHaveLength(1);
			expect(options[0]).toHaveTextContent("johndoe/backend-api");
		});
	},
};

export const SelectWorkspaceViaSearch: Story = {
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: mockWorkspaces,
			count: mockWorkspaces.length,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			const trigger = canvas.getByText("Workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(canvas.getByText("Workspace").closest("button")!);

		const body = within(canvasElement.ownerDocument.body);
		await body.findByRole("dialog");

		// Search for "janedoe" and select the result.
		const searchInput = body.getByPlaceholderText("Search workspaces...");
		await userEvent.type(searchInput, "janedoe");

		await waitFor(() => {
			expect(body.getAllByRole("option")).toHaveLength(1);
		});

		await userEvent.click(body.getByRole("option", { name: /janedoe/ }));

		// The trigger should now show the selected workspace.
		await waitFor(() => {
			expect(canvas.getByText("janedoe/my-project")).toBeInTheDocument();
		});
	},
};

export const UsageLimitExceeded: Story = {
	args: {
		...defaultArgs,
		createError: Object.assign(
			new Error("Request failed with status code 409"),
			{
				isAxiosError: true,
				response: {
					status: 409,
					statusText: "Conflict",
					data: {
						message: "Chat usage limit exceeded.",
						spent_micros: 900_000,
						limit_micros: 500_000,
						resets_at: "2026-03-16T00:00:00Z",
					},
					headers: {},
					config: {},
				},
				config: {},
				toJSON: () => ({}),
			},
		),
	},
};
