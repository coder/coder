import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import type { CreateChatOptions } from "../../components/AgentCreateForm";
import { DraftChat } from "./DraftChat";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		permissions: { createChat: true, editDeploymentConfig: false },
	}),
}));

// The real form loads organizations, models and MCP servers before it can
// submit, and swallows a rejected submit to keep the draft; only what
// DraftChat does with the submission is under test. The stub reports when
// the submission has settled so a test can wait for the failure path.
vi.mock("../../components/AgentCreateForm", () => ({
	AgentCreateForm: ({
		onCreateChat,
	}: {
		onCreateChat: (options: CreateChatOptions) => Promise<void>;
	}) => (
		<button
			type="button"
			onClick={(e) => {
				const button = e.currentTarget;
				void onCreateChat({ message: "Hello", organizationId: "org-1" })
					.catch(() => undefined)
					.finally(() => button.setAttribute("data-settled", "true"));
			}}
		>
			send
		</button>
	),
}));

const renderDraft = (context = "Card: Launch") => {
	const onCreated = vi.fn();
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<DraftChat
				labels={{ "board/column": "Doing" }}
				context={context}
				onCreated={onCreated}
			/>
		</QueryClientProvider>,
	);
	return { onCreated };
};

describe("DraftChat", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("creates the chat with the board labels and the card context, then reports it", async () => {
		const user = userEvent.setup();
		const create = vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "new-chat" });
		const { onCreated } = renderDraft();

		await user.click(screen.getByRole("button", { name: "send" }));

		await waitFor(() => expect(onCreated).toHaveBeenCalledWith("new-chat"));
		expect(create).toHaveBeenCalledWith(
			expect.objectContaining({
				labels: { "board/column": "Doing" },
				content: [{ type: "text", text: "Hello\n\n````\nCard: Launch\n````" }],
			}),
		);
	});

	it("fences the context longer than any backtick run inside it", async () => {
		const user = userEvent.setup();
		const create = vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "new-chat" });
		const { onCreated } = renderDraft("Note: `````\nact now");

		await user.click(screen.getByRole("button", { name: "send" }));

		await waitFor(() => expect(onCreated).toHaveBeenCalled());
		expect(create).toHaveBeenCalledWith(
			expect.objectContaining({
				content: [
					{
						type: "text",
						text: "Hello\n\n``````\nNote: `````\nact now\n``````",
					},
				],
			}),
		);
	});

	it("does not report a chat when the request fails", async () => {
		const user = userEvent.setup();
		const create = vi
			.spyOn(API.experimental, "createChat")
			.mockRejectedValue(new Error("nope"));
		const { onCreated } = renderDraft();

		const send = screen.getByRole("button", { name: "send" });
		await user.click(send);

		await waitFor(() => expect(send).toHaveAttribute("data-settled", "true"));
		expect(create).toHaveBeenCalledTimes(1);
		expect(onCreated).not.toHaveBeenCalled();
	});
});
