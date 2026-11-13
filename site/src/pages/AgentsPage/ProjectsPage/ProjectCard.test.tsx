import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import type { ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockArchivedChatProject,
	MockChatProject,
	MockChatProjectSummary,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { ArchivedProjectCard, ProjectCard } from "./ProjectCard";

const renderCard = (overrides: Partial<ChatProjectSummary> = {}) => {
	const callbacks = {
		onPin: vi.fn(),
		onUnpin: vi.fn(),
		onEdit: vi.fn(),
		onArchive: vi.fn(),
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

	// In a browser, a dialog that opens before the menu has finished closing
	// loses focus to the menu.
	it.each([
		["Edit project", "onEdit"],
		["Archive project", "onArchive"],
		["Delete project", "onDelete"],
	] as const)("opens %s once the menu has closed", async (item, callback) => {
		vi.useFakeTimers();
		onTestFinished(() => {
			vi.useRealTimers();
		});
		const user = userEvent.setup({ delay: null });
		const callbacks = renderCard();
		const actions = screen.getByRole("button", { name: /^Open project/ });
		await user.click(actions);
		await user.click(screen.getByRole("menuitem", { name: item }));
		expect(callbacks[callback]).not.toHaveBeenCalled();
		act(() => vi.runOnlyPendingTimers());
		expect(callbacks[callback]).toHaveBeenCalledExactlyOnceWith(actions);
		expect(actions).not.toHaveFocus();
	});

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

describe("ArchivedProjectCard", () => {
	// In a browser, a dialog that opens before the menu has finished closing
	// loses focus to the menu.
	it("opens Delete project once the menu has closed", async () => {
		vi.useFakeTimers();
		onTestFinished(() => {
			vi.useRealTimers();
		});
		const user = userEvent.setup({ delay: null });
		const onDelete = vi.fn();
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<MemoryRouter>
					<ArchivedProjectCard
						project={MockArchivedChatProject}
						to="/agents/projects/chat-project-1"
						onUnarchived={vi.fn()}
						onDelete={onDelete}
					/>
				</MemoryRouter>
			</QueryClientProvider>,
		);
		const actions = screen.getByRole("button", { name: /^Open project/ });
		await user.click(actions);
		await user.click(screen.getByRole("menuitem", { name: "Delete project" }));
		expect(onDelete).not.toHaveBeenCalled();
		act(() => vi.runOnlyPendingTimers());
		expect(onDelete).toHaveBeenCalledExactlyOnceWith(actions);
	});
});
