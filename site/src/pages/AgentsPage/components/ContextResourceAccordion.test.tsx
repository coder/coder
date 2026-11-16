import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	ContextResourceAccordion,
	ContextResourceAccordionContent,
	ContextResourceAccordionTrigger,
} from "./ContextResourceAccordion";

describe("ContextResourceAccordion", () => {
	it("expands and collapses with the keyboard and pointer", async () => {
		const user = userEvent.setup();
		const onOpenChange = vi.fn();
		render(
			<ContextResourceAccordion onOpenChange={onOpenChange}>
				<ContextResourceAccordionTrigger>
					Context files
				</ContextResourceAccordionTrigger>
				<ContextResourceAccordionContent>
					AGENTS.md
				</ContextResourceAccordionContent>
			</ContextResourceAccordion>,
		);
		await user.tab();
		await user.keyboard("{Enter}");
		expect(onOpenChange).toHaveBeenLastCalledWith(true);
		await user.click(screen.getByRole("button", { name: "Context files" }));
		expect(onOpenChange).toHaveBeenLastCalledWith(false);
	});
});
