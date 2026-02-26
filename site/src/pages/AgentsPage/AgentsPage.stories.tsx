import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { useRef } from "react";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { AgentsEmptyState } from "./AgentsPage";

const modelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const behaviorStorageKey = "agents.system-prompt";

/**
 * Wrapper that creates the top-bar actions ref that AgentsEmptyState
 * portals its admin button into.
 */
const AgentsEmptyStateWithPortal = (
	props: Omit<
		React.ComponentProps<typeof AgentsEmptyState>,
		"topBarActionsRef"
	>,
) => {
	const topBarActionsRef = useRef<HTMLDivElement>(null);
	return (
		<>
			<div
				ref={topBarActionsRef}
				data-testid="topbar-actions-host"
				className="flex items-center gap-2"
			/>
			<AgentsEmptyState {...props} topBarActionsRef={topBarActionsRef} />
		</>
	);
};

const meta: Meta<typeof AgentsEmptyStateWithPortal> = {
	title: "pages/AgentsPage/AgentsEmptyState",
	component: AgentsEmptyStateWithPortal,
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
		canSetSystemPrompt: true,
		canManageChatModelConfigs: false,
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
type Story = StoryObj<typeof AgentsEmptyStateWithPortal>;

export const Default: Story = {};

export const SavesBehaviorPromptAndRestores: Story = {
	play: async ({ canvasElement }) => {
		const host = canvasElement.ownerDocument.querySelector(
			'[data-testid="topbar-actions-host"]',
		)!;

		// Open the admin dialog via the portalled button.
		await userEvent.click(
			await within(host as HTMLElement).findByRole("button", {
				name: "Admin",
			}),
		);

		const dialog = await screen.findByRole("dialog");
		const textarea = await within(dialog).findByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);

		await userEvent.type(textarea, "You are a focused coding assistant.");
		await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(localStorage.getItem(behaviorStorageKey)).toBe(
				"You are a focused coding assistant.",
			);
		});
	},
};

export const UsesSavedBehaviorPromptOnSend: Story = {
	play: async ({ canvasElement, args }) => {
		const host = canvasElement.ownerDocument.querySelector(
			'[data-testid="topbar-actions-host"]',
		)!;

		// First, save a behavior prompt.
		await userEvent.click(
			await within(host as HTMLElement).findByRole("button", {
				name: "Admin",
			}),
		);

		const dialog = await screen.findByRole("dialog");
		const textarea = await within(dialog).findByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);

		await userEvent.type(textarea, "Use concise and actionable answers.");
		await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));

		// Modify without saving, then close.
		await userEvent.clear(textarea);
		await userEvent.type(textarea, "Unsaved draft prompt");
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Close" }),
		);

		// Wait for the dialog to fully close (exit animation) before
		// interacting with the page content underneath.
		await waitFor(() => {
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		});

		// Type a chat message and send.
		await userEvent.type(
			screen.getByPlaceholderText(
				"Ask Coder to build, fix bugs, or explore your project...",
			),
			"Create a README checklist",
		);
		await userEvent.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => {
			expect(args.onCreateChat).toHaveBeenCalledWith(
				expect.objectContaining({
					message: "Create a README checklist",
				}),
			);
		});
	},
};
