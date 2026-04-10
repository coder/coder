import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockWorkspace } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { AgentCreateForm } from "./AgentCreateForm";

const modelConfigID = "model-config-1";

const modelOptions = [
	{
		id: modelConfigID,
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const mock403Error = Object.assign(
	new Error("Request failed with status code 403"),
	{
		isAxiosError: true,
		response: {
			status: 403,
			statusText: "Forbidden",
			data: {
				message: "Forbidden.",
				detail: "Insufficient permissions to use Coder Agents.",
			},
			headers: {},
			config: {},
		},
		config: {},
		toJSON: () => ({}),
	},
);

const meta: Meta<typeof AgentCreateForm> = {
	title: "pages/AgentsPage/AgentCreateForm",
	component: AgentCreateForm,
	decorators: [withDashboardProvider],
	args: {
		onCreateChat: fn(),
		isCreating: false,
		createError: undefined,
		canCreateChat: true,
		modelCatalog: null,
		modelOptions: [...modelOptions],
		isModelCatalogLoading: false,
		modelConfigs: [],
		isModelConfigsLoading: false,
		workspaceCount: 0,
		workspaceOptions: [],
		workspacesError: undefined,
		isWorkspacesLoading: false,
	},
	beforeEach: () => {
		localStorage.clear();
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
	args: {
		workspaceOptions: mockWorkspaces,
		workspaceCount: mockWorkspaces.length,
	},
	beforeEach: () => {
		localStorage.clear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		// Open the "+" menu first, then click the workspace trigger inside it.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			const trigger = body.getByText("Attach workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(
			body.getByText("Attach workspace").closest("button")!,
		);
		// Wait for the workspace combobox dropdown to appear so
		// Chromatic captures it.
		await body.findByPlaceholderText("Search workspaces...");
	},
};

export const SearchWorkspaces: Story = {
	args: {
		workspaceOptions: mockWorkspaces,
		workspaceCount: mockWorkspaces.length,
	},
	beforeEach: () => {
		localStorage.clear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		// Open the "+" menu first, then click the workspace trigger inside it.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			const trigger = body.getByText("Attach workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(
			body.getByText("Attach workspace").closest("button")!,
		);

		// Type in the search input to filter workspaces.
		const searchInput = body.getByPlaceholderText("Search workspaces...");
		await userEvent.type(searchInput, "backend");

		// Only the matching workspace should remain visible.
		await waitFor(() => {
			const options = body.getAllByRole("option");
			// "Auto-create Workspace" is filtered out, only
			// "johndoe/backend-api" matches.
			expect(options).toHaveLength(1);
			expect(options[0]).toHaveTextContent("backend-api");
		});
	},
};

export const SelectWorkspaceViaSearch: Story = {
	args: {
		workspaceOptions: mockWorkspaces,
		workspaceCount: mockWorkspaces.length,
	},
	beforeEach: () => {
		localStorage.clear();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		// Open the "+" menu first, then click the workspace trigger inside it.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			const trigger = body.getByText("Attach workspace").closest("button")!;
			expect(trigger).toBeEnabled();
		});
		await userEvent.click(
			body.getByText("Attach workspace").closest("button")!,
		);

		// Search for "backend" and select the result.
		const searchInput = body.getByPlaceholderText("Search workspaces...");
		await userEvent.type(searchInput, "backend");

		await waitFor(() => {
			expect(body.getAllByRole("option")).toHaveLength(1);
		});

		await userEvent.click(body.getByRole("option", { name: /backend-api/ }));

		// Re-open the "+" menu to verify the selected workspace label.
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await waitFor(() => {
			expect(body.getByText("backend-api")).toBeInTheDocument();
		});
	},
};

export const LoadingModelCatalog: Story = {
	args: {
		...defaultArgs,
		modelCatalog: null,
		modelOptions: [],
		isModelCatalogLoading: true,
		isModelConfigsLoading: true,
	},
};

export const NoModelsConfigured: Story = {
	args: {
		...defaultArgs,
		modelCatalog: { providers: [] },
		modelOptions: [],
		isModelCatalogLoading: false,
		isModelConfigsLoading: false,
	},
};

export const PreservesAttachmentsOnFailedSend: Story = {
	args: {
		...defaultArgs,
		onCreateChat: fn().mockRejectedValue(new Error("server error")),
	},
	beforeEach: () => {
		localStorage.clear();
		// Pre-persist an uploaded attachment so it is restored on mount.
		localStorage.setItem(
			"agents.persisted-attachments",
			JSON.stringify([
				{
					fileId: "persisted-file-1",
					fileName: "photo.png",
					fileType: "image/png",
					lastModified: 1000,
				},
			]),
		);
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// The restored attachment should appear on mount.
		await waitFor(() => {
			expect(canvas.getByLabelText("Remove photo.png")).toBeInTheDocument();
		});

		// Type a message and submit.
		const input = canvas.getByTestId("chat-message-input");
		await userEvent.click(input);
		await userEvent.keyboard("test message");
		await userEvent.click(canvas.getByRole("button", { name: "Send" }));

		// Wait for onCreateChat to have been called (and rejected).
		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalled();
		});

		// The attachment must still be visible after the failed send.
		await waitFor(() => {
			expect(canvas.getByLabelText("Remove photo.png")).toBeInTheDocument();
		});

		// localStorage must still have the persisted attachment.
		const stored = localStorage.getItem("agents.persisted-attachments");
		expect(stored).not.toBeNull();
		const parsed = JSON.parse(stored!);
		expect(parsed).toHaveLength(1);
		expect(parsed[0].fileId).toBe("persisted-file-1");
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

export const ForbiddenErrorWithRole: Story = {
	args: {
		...defaultArgs,
		canCreateChat: true,
		createError: mock403Error,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The friendly "role required" alert must NOT appear because the
		// user has the agents-access role.
		await expect(
			canvas.queryByText("Permission required"),
		).not.toBeInTheDocument();
		// The generic ErrorAlert should surface the real backend message.
		await expect(canvas.getByText("Forbidden.")).toBeInTheDocument();
		// The textbox should remain enabled since the user has the role.
		const textbox = canvas.getByRole("textbox");
		await expect(textbox).not.toHaveAttribute("aria-disabled", "true");
	},
};

export const ForbiddenNoAgentsRole: Story = {
	args: {
		...defaultArgs,
		canCreateChat: false,
		createError: mock403Error,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Permission required")).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /View Docs/ }),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("heading", { name: "Forbidden." }),
		).not.toBeInTheDocument();
		// The textarea should be disabled so the user cannot
		// accidentally trigger the generic error.
		const textbox = canvas.getByRole("textbox");
		await expect(textbox).toHaveAttribute("aria-disabled", "true");
	},
};
