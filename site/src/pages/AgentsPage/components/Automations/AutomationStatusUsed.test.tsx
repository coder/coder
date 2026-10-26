import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockWebhookChatAutomation } from "#/testHelpers/chatEntities";
import { AutomationEnabledSwitch, AutomationNextRun } from "./AutomationStatus";

// The server turns a single-use webhook off when it accepts its event.
const usedWebhook = {
	...MockWebhookChatAutomation,
	webhook_use: "single" as const,
	webhook_consumed_at: "2026-10-13T09:00:00Z",
	enabled: false,
};

describe("a used single-use webhook", () => {
	it("reads Used, not Disabled, in the list", () => {
		render(
			<TooltipProvider>
				<AutomationNextRun automation={usedWebhook} />
			</TooltipProvider>,
		);
		expect(screen.getByText("Used")).toBeInTheDocument();
		expect(screen.queryByText("Disabled")).not.toBeInTheDocument();
	});

	it("labels its switch Used and explains it cannot run again", () => {
		const onToggle = vi.fn();
		render(
			<TooltipProvider>
				<AutomationEnabledSwitch
					automation={usedWebhook}
					canUpdate
					isUpdating={false}
					onToggle={onToggle}
					showLabel
				/>
			</TooltipProvider>,
		);
		expect(screen.getByText("Used")).toBeInTheDocument();
		expect(screen.queryByText("Disabled")).not.toBeInTheDocument();
		const toggle = screen.getByRole("switch", {
			name: `Enable ${usedWebhook.name}`,
		});
		expect(toggle).toHaveAttribute("aria-disabled", "true");
		expect(toggle).toHaveAccessibleDescription(
			"A used single-use webhook cannot run again.",
		);
	});
});
