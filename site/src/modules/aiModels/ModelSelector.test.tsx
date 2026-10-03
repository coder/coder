import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
import { ModelSelector } from "./ModelSelector";
import { MockModelSelectorOption } from "./modelSelectorFixtures";

const originalMatchMedia = window.matchMedia;

const stubMobileViewport = (belowMd: boolean) => {
	vi.stubGlobal("matchMedia", (query: string) => {
		const result = originalMatchMedia(query);

		return query === belowMdViewportMediaQuery
			? { ...result, matches: belowMd }
			: result;
	});
};

const Composer = ({
	onValueChange,
	docked = true,
}: {
	onValueChange: (value: string) => void;
	docked?: boolean;
}) => {
	const [composer, setComposer] = useState<HTMLDivElement | null>(null);

	return (
		<div ref={setComposer}>
			<ModelSelector
				options={[MockModelSelectorOption]}
				value=""
				onValueChange={onValueChange}
				mobileAnchor={docked ? composer : undefined}
			/>
		</div>
	);
};

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("ModelSelector", () => {
	it.each([
		{ label: "on desktop", belowMd: false, docked: true },
		{ label: "without a composer anchor", belowMd: true, docked: false },
	])("selects a model $label", async ({ belowMd, docked }) => {
		stubMobileViewport(belowMd);
		const user = userEvent.setup();
		const onValueChange = vi.fn();

		renderComponent(<Composer onValueChange={onValueChange} docked={docked} />);

		await user.click(screen.getByRole("combobox", { name: "Select model" }));
		await user.click(await screen.findByRole("option", { name: /gpt-4o/ }));

		expect(onValueChange).toHaveBeenCalledExactlyOnceWith(
			MockModelSelectorOption.id,
		);
	});

	it("accepts an opening click captured before the composer anchor becomes available", async () => {
		stubMobileViewport(true);
		const user = userEvent.setup();
		const onValueChange = vi.fn();

		const view = renderComponent(
			<Composer onValueChange={onValueChange} docked={false} />,
		);
		const trigger = screen.getByRole("combobox", { name: "Select model" });

		view.rerender(<Composer onValueChange={onValueChange} />);

		await user.click(trigger);
		await screen.findByRole("combobox", { name: "Search models" });
		await user.keyboard(`${MockModelSelectorOption.model}{Enter}`);

		expect(onValueChange).toHaveBeenCalledExactlyOnceWith(
			MockModelSelectorOption.id,
		);
	});
});
