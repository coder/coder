import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act, useState } from "react";
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

// Excludes style writes from Radix and the theme provider.
const geometryCallCount = (calls: readonly [string, ...unknown[]][]) =>
	calls.filter(([name]) => name.startsWith("--anchored-overlay-")).length;

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

const openPicker = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("combobox", { name: "Select model" }));

	return screen.findByRole("combobox", { name: "Search models" });
};

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("ModelSelector", () => {
	it.each([
		{ label: "on desktop", belowMd: false, docked: true },
		{ label: "without a composer anchor", belowMd: true, docked: false },
	])(
		"keeps mobile positioning inactive $label",
		async ({ belowMd, docked }) => {
			stubMobileViewport(belowMd);
			const user = userEvent.setup();
			const onValueChange = vi.fn();
			const styleWrite = vi.spyOn(CSSStyleDeclaration.prototype, "setProperty");

			renderComponent(
				<Composer onValueChange={onValueChange} docked={docked} />,
			);

			await openPicker(user);
			expect(geometryCallCount(styleWrite.mock.calls)).toBe(0);

			await user.click(screen.getByRole("option", { name: /gpt-4o/ }));

			expect(onValueChange).toHaveBeenCalledExactlyOnceWith(
				MockModelSelectorOption.id,
			);
		},
	);

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

	it.each([false, true])(
		"coalesces open-menu geometry work and stops it on close (pending frame: %s)",
		async (pendingFrame) => {
			stubMobileViewport(true);
			const viewport = Object.assign(new EventTarget(), {
				offsetTop: 0,
				offsetLeft: 0,
				width: 390,
				height: 800,
				pageTop: 0,
				pageLeft: 0,
				scale: 1,
			});
			vi.stubGlobal("visualViewport", viewport);

			const user = userEvent.setup();
			const rootWrite = vi.spyOn(document.documentElement.style, "setProperty");
			const styleWrite = vi.spyOn(CSSStyleDeclaration.prototype, "setProperty");

			renderComponent(<Composer onValueChange={vi.fn()} />);

			await user.tab();
			expect(geometryCallCount(styleWrite.mock.calls)).toBe(0);

			const search = await openPicker(user);
			expect(geometryCallCount(styleWrite.mock.calls)).toBeGreaterThan(0);

			vi.useFakeTimers({
				toFake: ["requestAnimationFrame", "cancelAnimationFrame"],
			});
			const requestFrame = vi.spyOn(window, "requestAnimationFrame");

			await user.type(search, "gpt");

			act(() => {
				screen.getByRole("listbox").dispatchEvent(new Event("scroll"));
			});

			expect(requestFrame).not.toHaveBeenCalled();

			const dispatchGeometryEvents = () => {
				viewport.dispatchEvent(new Event("resize"));
				viewport.dispatchEvent(new Event("scroll"));
				window.dispatchEvent(new Event("resize"));
				document.body.dispatchEvent(new Event("scroll"));
			};

			act(dispatchGeometryEvents);
			expect(requestFrame).toHaveBeenCalledTimes(1);
			expect(vi.getTimerCount()).toBe(1);

			act(() => {
				vi.advanceTimersToNextFrame();
			});

			expect(vi.getTimerCount()).toBe(0);

			act(dispatchGeometryEvents);
			expect(requestFrame).toHaveBeenCalledTimes(2);
			expect(vi.getTimerCount()).toBe(1);

			if (!pendingFrame) {
				act(() => {
					vi.advanceTimersToNextFrame();
				});
			}

			await user.keyboard("{Escape}");
			expect(vi.getTimerCount()).toBe(0);

			requestFrame.mockClear();
			act(dispatchGeometryEvents);

			expect(requestFrame).not.toHaveBeenCalled();

			styleWrite.mockClear();
			await openPicker(user);
			expect(geometryCallCount(styleWrite.mock.calls)).toBeGreaterThan(0);

			act(dispatchGeometryEvents);
			expect(requestFrame).toHaveBeenCalledTimes(1);

			await user.keyboard("{Escape}");
			expect(vi.getTimerCount()).toBe(0);
			expect(rootWrite).not.toHaveBeenCalled();
		},
	);
});
