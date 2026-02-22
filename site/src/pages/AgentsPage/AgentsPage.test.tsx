import { render } from "testHelpers/renderHelpers";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AgentsEmptyState } from "./AgentsPage";

const behaviorStorageKey = "agents.system-prompt";

const modelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const renderEmptyState = (onCreateChat: (message: string) => Promise<void>) => {
	const topBarActionsHost = document.createElement("div");
	document.body.append(topBarActionsHost);
	const topBarActionsRef = {
		current: topBarActionsHost,
	};

	const renderResult = render(
		<AgentsEmptyState
			onCreateChat={async (options) => onCreateChat(options.systemPrompt ?? "")}
			isCreating={false}
			createError={undefined}
			modelCatalog={null}
			modelOptions={modelOptions}
			isModelCatalogLoading={false}
			modelCatalogError={undefined}
			canSetSystemPrompt
			canManageChatModelConfigs={false}
			canUseLocalWorkspaceMode={false}
			topBarActionsRef={topBarActionsRef}
		/>,
	);

	return { ...renderResult, topBarActionsHost };
};

describe("Agents behavior settings", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("saves behavior prompt changes and restores them on remount", async () => {
		const user = userEvent.setup();
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const { unmount, topBarActionsHost } = renderEmptyState(onCreateChat);

		await user.click(
			await within(topBarActionsHost).findByRole("button", { name: "Admin" }),
		);
		const dialog = await screen.findByRole("dialog");
		const textarea = await within(dialog).findByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);

		await user.type(textarea, "You are a focused coding assistant.");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		expect(localStorage.getItem(behaviorStorageKey)).toBe(
			"You are a focused coding assistant.",
		);

		unmount();
		topBarActionsHost.remove();

		const rerenderUser = userEvent.setup();
		const { topBarActionsHost: rerenderedActionsHost } =
			renderEmptyState(onCreateChat);
		await rerenderUser.click(
			await within(rerenderedActionsHost).findByRole("button", {
				name: "Admin",
			}),
		);
		const rerenderedDialog = await screen.findByRole("dialog");
		const rerenderedTextarea = await within(
			rerenderedDialog,
		).findByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);
		expect(rerenderedTextarea).toHaveValue(
			"You are a focused coding assistant.",
		);
	});

	it("uses the saved behavior prompt in the existing create-chat path", async () => {
		const user = userEvent.setup();
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const { topBarActionsHost } = renderEmptyState(onCreateChat);

		await user.click(
			await within(topBarActionsHost).findByRole("button", { name: "Admin" }),
		);
		const dialog = await screen.findByRole("dialog");
		const textarea = await within(dialog).findByPlaceholderText(
			"Optional. Set deployment-wide instructions for all new chats.",
		);

		await user.type(textarea, "Use concise and actionable answers.");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));
		await user.clear(textarea);
		await user.type(textarea, "Unsaved draft prompt");
		await user.click(within(dialog).getByRole("button", { name: "Close" }));

		await user.type(
			screen.getByPlaceholderText(
				"Ask Coder to build, fix bugs, or explore your project...",
			),
			"Create a README checklist",
		);
		await user.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => {
			expect(onCreateChat).toHaveBeenCalledWith(
				"Use concise and actionable answers.",
			);
		});

		topBarActionsHost.remove();
	});
});
