import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

const usage = {
	usedTokens: 26_000,
	contextLimitTokens: 1_100_000,
	compressionThreshold: 70,
};

describe("ContextUsageIndicator", () => {
	it("opens Details using only the keyboard and passes its surviving opener", async () => {
		const user = userEvent.setup();
		const onOpenDetails = vi.fn();
		render(
			<ContextUsageIndicator usage={usage} onOpenDetails={onOpenDetails} />,
		);
		await user.tab();
		const trigger = screen.getByRole("button", {
			name: "Context usage: 2% - 26K / 1.1M context used; Compacts at 70%",
		});
		expect(trigger).toHaveFocus();
		await user.keyboard("{Enter}");
		await user.tab();
		expect(screen.getByRole("button", { name: "Details" })).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(onOpenDetails).toHaveBeenCalledExactlyOnceWith(trigger);
	});
	it("Escape restores focus without reopening, then allows deliberate activation", async () => {
		const user = userEvent.setup();
		render(
			<>
				<ContextUsageIndicator usage={usage} onOpenDetails={vi.fn()} />
				<button type="button">After</button>
			</>,
		);
		await user.tab();
		const trigger = screen.getByRole("button", { name: /Context usage:/ });
		await user.keyboard(" ");
		await user.tab();
		await user.keyboard("{Escape}");
		expect(trigger).toHaveFocus();
		await user.tab();
		expect(screen.getByRole("button", { name: "After" })).toHaveFocus();
		await user.tab({ shift: true });
		await user.keyboard("{Enter}");
		await user.tab();
		expect(screen.getByRole("button", { name: "Details" })).toHaveFocus();
	});
	it("does not steal hover focus and allows pointer transfer to Details", async () => {
		const user = userEvent.setup();
		const onOpenDetails = vi.fn();
		render(
			<>
				<button type="button">Before</button>
				<ContextUsageIndicator usage={usage} onOpenDetails={onOpenDetails} />
			</>,
		);
		await user.tab();
		const before = screen.getByRole("button", { name: "Before" });
		const trigger = screen.getByRole("button", { name: /Context usage:/ });
		await user.hover(trigger);
		expect(before).toHaveFocus();
		await user.hover(screen.getByRole("button", { name: "Details" }));
		await user.click(screen.getByRole("button", { name: "Details" }));
		expect(onOpenDetails).toHaveBeenCalledExactlyOnceWith(trigger);
	});
	it("does not trap forward or backward tabbing", async () => {
		const user = userEvent.setup();
		render(
			<>
				<button type="button">Before</button>
				<ContextUsageIndicator usage={usage} onOpenDetails={vi.fn()} />
				<button type="button">After</button>
			</>,
		);
		await user.tab();
		await user.tab();
		await user.keyboard("{Enter}");
		await user.tab();
		expect(screen.getByRole("button", { name: "Details" })).toHaveFocus();
		await user.tab({ shift: true });
		expect(
			screen.getByRole("button", { name: /Context usage:/ }),
		).toHaveFocus();
		await user.tab({ shift: true });
		expect(screen.getByRole("button", { name: "Before" })).toHaveFocus();
	});
});
