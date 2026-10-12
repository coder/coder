import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatQueuedMessage } from "#/testHelpers/chatEntities";
import { QueuedMessagesList } from "./QueuedMessagesList";

const renderList = (readOnly: boolean) => {
	const onDelete = vi.fn();
	const onPromote = vi.fn();
	render(
		<TooltipProvider>
			<QueuedMessagesList
				messages={[MockChatQueuedMessage]}
				automationReferences={{ references: new Map(), status: "settled" }}
				onDelete={onDelete}
				onPromote={onPromote}
				readOnly={readOnly}
			/>
		</TooltipProvider>,
	);
	return { onDelete, onPromote };
};

describe("QueuedMessagesList actions", () => {
	it("removes a queued message for a viewer who can change the chat", async () => {
		const { onDelete } = renderList(false);
		await userEvent.click(
			screen.getByRole("button", { name: "Remove from queue" }),
		);
		expect(onDelete).toHaveBeenCalledWith(MockChatQueuedMessage.id);
	});

	it("offers a read-only viewer no queue actions", () => {
		renderList(true);
		expect(
			screen.queryAllByRole("button", { name: /Remove from queue|Send now/ }),
		).toEqual([]);
	});
});
