import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Slider } from "./Slider";

describe("Slider", () => {
	beforeEach(() => {
		// Edge-aligned thumbs need a nonzero track size, which jsdom cannot measure.
		vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
			new DOMRect(0, 0, 100, 16),
		);
	});
	afterEach(() => vi.restoreAllMocks());
	it("reports and commits array values after keyboard changes", async () => {
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		const onValueCommit = vi.fn();
		render(
			<Slider
				aria-label="Volume"
				defaultValue={[20]}
				step={5}
				onValueChange={onValueChange}
				onValueCommit={onValueCommit}
			/>,
		);
		await screen.findByRole("slider", { name: "Volume" });
		await user.tab();
		await user.keyboard("{ArrowRight}");
		expect(onValueChange).toHaveBeenCalledWith([25], expect.anything());
		expect(onValueCommit).toHaveBeenCalledWith([25], expect.anything());
	});

	it("keeps range thumbs separated by the configured number of steps", async () => {
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		render(
			<Slider
				defaultValue={[20, 30]}
				step={5}
				minStepsBetweenThumbs={1}
				onValueChange={onValueChange}
			/>,
		);
		await screen.findAllByRole("slider");
		await user.tab();
		await user.keyboard("{ArrowRight}{ArrowRight}");
		expect(onValueChange).toHaveBeenCalledTimes(1);
		expect(onValueChange).toHaveBeenCalledWith([25, 30], expect.anything());
	});

	it("ignores keyboard interaction when disabled", async () => {
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		render(
			<Slider defaultValue={[20]} disabled onValueChange={onValueChange} />,
		);
		await screen.findAllByRole("slider");
		await user.tab();
		await user.keyboard("{ArrowRight}");
		expect(onValueChange).not.toHaveBeenCalled();
	});
});
