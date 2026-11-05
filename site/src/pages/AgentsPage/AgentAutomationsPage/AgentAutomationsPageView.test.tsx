import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";
import { placeholderAutomations } from "./placeholderAutomations";

describe("AgentAutomationsPageView", () => {
	it.each(placeholderAutomations)("toggles $name", async (automation) => {
		const user = userEvent.setup();
		const onEnabledChange = vi.fn();
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<ThemeOverride theme={themes[DEFAULT_THEME]}>
					<MemoryRouter>
						<AgentAutomationsPageView
							automations={[automation]}
							onEnabledChange={onEnabledChange}
							onEditAutomation={vi.fn()}
							onRunAutomation={vi.fn()}
							onViewChats={vi.fn()}
							onDeleteAutomation={vi.fn()}
						/>
					</MemoryRouter>
				</ThemeOverride>
			</QueryClientProvider>,
		);
		await user.click(
			screen.getByRole("switch", { name: `Enable ${automation.name}` }),
		);
		expect(onEnabledChange).toHaveBeenCalledExactlyOnceWith(
			automation.id,
			!automation.enabled,
		);
	});

	it.each([
		["Run now", "run"],
		["View chats", "viewChats"],
		["Edit", "edit"],
		["Delete…", "delete"],
	] as const)("handles %s", async (itemLabel, action) => {
		const user = userEvent.setup();
		const automation = placeholderAutomations[0];
		if (!automation) {
			throw new Error("Expected a placeholder automation.");
		}
		const callbacks = {
			edit: vi.fn(),
			run: vi.fn(),
			viewChats: vi.fn(),
			delete: vi.fn(),
		};
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<ThemeOverride theme={themes[DEFAULT_THEME]}>
					<MemoryRouter>
						<AgentAutomationsPageView
							automations={[automation]}
							onEnabledChange={vi.fn()}
							onEditAutomation={callbacks.edit}
							onRunAutomation={callbacks.run}
							onViewChats={callbacks.viewChats}
							onDeleteAutomation={callbacks.delete}
						/>
					</MemoryRouter>
				</ThemeOverride>
			</QueryClientProvider>,
		);

		await user.click(
			screen.getByRole("button", {
				name: `More actions for ${automation.name}`,
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: itemLabel }));

		expect(callbacks[action]).toHaveBeenCalledExactlyOnceWith(automation);
	});
});
