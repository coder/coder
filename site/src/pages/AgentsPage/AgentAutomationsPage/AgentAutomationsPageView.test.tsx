import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { ChatAutomation } from "#/api/typesGenerated";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";

const mockPausedAutomation: ChatAutomation = {
	...MockChatAutomation,
	id: "paused-review",
	name: "Paused review",
	owner: null,
	paused_reasons: ["model_unavailable"],
};

const renderView = (
	onDeleteAutomation: (automation: typeof MockChatAutomation) => void,
) =>
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<MemoryRouter>
					<AgentAutomationsPageView
						automations={[
							MockChatAutomation,
							MockWebhookChatAutomation,
							mockPausedAutomation,
						]}
						organizations={[MockDefaultOrganization]}
						isLoading={false}
						loadErrors={[]}
						onRetry={vi.fn()}
						onMenuTrigger={vi.fn()}
						onDeleteAutomation={onDeleteAutomation}
					/>
				</MemoryRouter>
			</ThemeOverride>
		</QueryClientProvider>,
	);

describe("AgentAutomationsPageView", () => {
	it.each([
		["status:paused", mockPausedAutomation],
		["trigger:webhook", MockWebhookChatAutomation],
		["Unknown", mockPausedAutomation],
		["status:enabled", MockChatAutomation],
	])(
		"filters %s before invoking the selected row action",
		async (filter, selected) => {
			const user = userEvent.setup();
			const onDeleteAutomation = vi.fn();
			renderView(onDeleteAutomation);
			await user.type(
				screen.getByRole("combobox", { name: /Search and filter automations/ }),
				`${filter}{Enter}`,
			);
			await user.click(
				screen.getByRole("button", { name: /More actions for/ }),
			);
			await user.click(screen.getByRole("menuitem", { name: "Delete…" }));
			await waitFor(() =>
				expect(onDeleteAutomation).toHaveBeenCalledExactlyOnceWith(selected),
			);
		},
	);
});
