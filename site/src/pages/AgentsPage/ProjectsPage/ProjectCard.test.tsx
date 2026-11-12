import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import type { ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSummary,
} from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ProjectCard } from "./ProjectCard";

const renderCard = (overrides: Partial<ChatProjectSummary> = {}) => {
	const callbacks = {
		onPin: vi.fn(),
		onUnpin: vi.fn(),
		onEdit: vi.fn(),
		onDelete: vi.fn(),
	};
	renderComponent(
		<MemoryRouter>
			<ProjectCard
				summary={{ ...MockChatProjectSummary, ...overrides }}
				to="/agents/projects/chat-project-1"
				pinPending={false}
				{...callbacks}
			/>
		</MemoryRouter>,
	);
	return callbacks;
};

describe("ProjectCard", () => {
	it.each<[string, Partial<ChatProjectSummary>, RegExp]>([
		["a private project", { member_count: 1 }, /Only you$/],
		["a shared project", { member_count: 1234 }, /1,234 members$/],
	])("describes %s", (_, overrides, description) => {
		renderCard(overrides);
		expect(
			screen.getByRole("link", { name: MockChatProject.name }),
		).toHaveAccessibleDescription(description);
	});

	it("unpins from the pin button and keeps focus in the card", async () => {
		const user = userEvent.setup();
		const { onUnpin } = renderCard();
		await user.click(
			screen.getByRole("button", { name: `Unpin ${MockChatProject.name}` }),
		);
		expect(onUnpin).toHaveBeenCalledOnce();
		expect(
			screen.getByRole("button", {
				name: `Open project actions for ${MockChatProject.name}`,
			}),
		).toHaveFocus();
	});

	it.each(["Edit project", "Delete project"])(
		"keeps focus off the actions button after picking %s with the mouse",
		async (item) => {
			vi.useFakeTimers();
			onTestFinished(() => {
				vi.useRealTimers();
			});
			const user = userEvent.setup({ delay: null });
			renderCard();
			const actions = screen.getByRole("button", { name: /^Open project/ });
			await user.click(actions);
			await user.click(screen.getByRole("menuitem", { name: item }));
			act(() => vi.runOnlyPendingTimers());
			expect(actions).not.toHaveFocus();
		},
	);

	it("pins from the actions menu with the keyboard", async () => {
		const user = userEvent.setup();
		const { onPin } = renderCard({ pinned: false, pin_order: 0 });
		// The link, then the actions button.
		await user.tab();
		await user.tab();
		await user.keyboard("{Enter}");
		await screen.findByRole("menuitem", { name: "Pin project" });
		await user.keyboard("{Enter}");
		expect(onPin).toHaveBeenCalledOnce();
	});
});
