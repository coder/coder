import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { belowMdViewportMediaQuery } from "#/utils/mobile";
import { ModelSelector, type ModelSelectorOption } from "./ModelSelector";
import { MockModelSelectorOption } from "./modelSelectorFixtures";

const gpt4o: ModelSelectorOption = {
	...MockModelSelectorOption,
	id: "openai/gpt-4o",
	model: "gpt-4o",
	displayName: "GPT-4o",
	contextLimit: 128_000,
};

const gpt4oMini: ModelSelectorOption = {
	...MockModelSelectorOption,
	id: "openai/gpt-4o-mini",
	model: "gpt-4o-mini",
	displayName: "GPT-4o Mini",
	contextLimit: 128_000,
};

const gpt5: ModelSelectorOption = {
	...MockModelSelectorOption,
	id: "openai/gpt-5",
	model: "gpt-5",
	displayName: "GPT-5",
	contextLimit: 400_000,
	reasoningEffortDefault: "medium",
	reasoningEfforts: [
		"none",
		"minimal",
		"low",
		"medium",
		"high",
		"xhigh",
		"max",
	],
};

const options = [gpt4o, gpt4oMini, gpt5];

// Captured once so stubs wrap the real implementation, not an earlier stub.
const originalMatchMedia = window.matchMedia;

const stubMediaQueries = ({
	belowMd,
	coarsePointer = false,
}: {
	belowMd: boolean;
	coarsePointer?: boolean;
}) => {
	vi.stubGlobal("matchMedia", (query: string) => {
		const result = originalMatchMedia(query);
		if (query === belowMdViewportMediaQuery) {
			return { ...result, matches: belowMd };
		}
		if (query === "(pointer: coarse)") {
			return { ...result, matches: coarsePointer };
		}
		return result;
	});
};

// jsdom has no visualViewport. A real EventTarget lets the test observe the
// hook's keyboard-tracking subscriptions being added and removed.
const stubVisualViewport = () => {
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
	return viewport;
};

// Spies on differently overloaded methods share only their recorded calls.
type CallSpy = { mock: { calls: ReadonlyArray<readonly unknown[]> } };

const listenersOf = (spy: CallSpy, type: string) =>
	spy.mock.calls
		.filter(([eventType]) => eventType === type)
		.map(([, listener]) => listener);

// Geometry variables are the only `--mobile-menu-*` writes in the app.
const geometryWriteCount = (spy: CallSpy) =>
	spy.mock.calls.filter(
		([name]) => typeof name === "string" && name.startsWith("--mobile-menu-"),
	).length;

type ComposerProps = Omit<
	React.ComponentProps<typeof ModelSelector>,
	"mobileAnchor" | "value" | "onValueChange"
> & {
	initialValue?: string;
	onValueChange: (value: string) => void;
	docked?: boolean;
};

// Stateful like the chat page: selection and effort changes feed back into
// props, and the picker docks to the composer box that contains it.
const Composer = ({
	initialValue = "",
	onValueChange,
	onReasoningEffortChange,
	reasoningEffort,
	docked = true,
	...props
}: ComposerProps) => {
	const [composer, setComposer] = useState<HTMLElement | null>(null);
	const [value, setValue] = useState(initialValue);
	const [effort, setEffort] = useState(reasoningEffort);
	return (
		<section aria-label="Composer" ref={setComposer}>
			<ModelSelector
				{...props}
				value={value}
				onValueChange={(next) => {
					onValueChange(next);
					setValue(next);
				}}
				reasoningEffort={effort}
				onReasoningEffortChange={
					onReasoningEffortChange &&
					((next) => {
						onReasoningEffortChange(next);
						setEffort(next);
					})
				}
				mobileAnchor={docked ? composer : undefined}
			/>
		</section>
	);
};

const findSearch = () =>
	screen.findByRole("combobox", { name: "Search models" });

const openPicker = async (
	user: ReturnType<typeof userEvent.setup>,
	name = "Select model",
) => {
	await user.click(screen.getByRole("combobox", { name }));
	return findSearch();
};

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("ModelSelector", () => {
	it("selects the model matched by keyboard search on a docked mobile picker", async () => {
		stubMediaQueries({ belowMd: true });
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		renderComponent(
			<Composer
				options={options}
				onValueChange={onValueChange}
				onReasoningEffortChange={vi.fn()}
			/>,
		);

		const search = await openPicker(user);
		expect(search).toHaveFocus();
		await user.keyboard("mini{Enter}");

		expect(onValueChange).toHaveBeenCalledTimes(1);
		expect(onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
		expect(await openPicker(user, "GPT-4o Mini")).toHaveValue("");
	});

	it("does not focus the search on coarse pointers so the keyboard stays closed", async () => {
		stubMediaQueries({ belowMd: true, coarsePointer: true });
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		renderComponent(
			<Composer options={options} onValueChange={onValueChange} />,
		);

		const search = await openPicker(user);
		expect(search).not.toHaveFocus();

		await user.click(screen.getByRole("option", { name: /GPT-4o Mini/ }));
		expect(onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	});

	it("keeps a model with efforts open and changes effort from the pinned row", async () => {
		stubMediaQueries({ belowMd: true });
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		const onReasoningEffortChange = vi.fn();
		renderComponent(
			<Composer
				options={options}
				onValueChange={onValueChange}
				reasoningEffort="medium"
				onReasoningEffortChange={onReasoningEffortChange}
			/>,
		);

		await openPicker(user);
		await user.keyboard("gpt-5{Enter}");
		expect(onValueChange).toHaveBeenCalledWith("openai/gpt-5");

		// The effort row sits outside the command list, so tabbing past the
		// info button reaches the slider and cmdk does not consume arrows.
		await user.tab();
		await user.tab();
		expect(screen.getByRole("slider")).toHaveFocus();
		await user.keyboard("{ArrowRight}");
		expect(onReasoningEffortChange).toHaveBeenLastCalledWith("high");
		await user.keyboard("{End}");
		expect(onReasoningEffortChange).toHaveBeenLastCalledWith("max");
		await user.keyboard("{Home}");
		expect(onReasoningEffortChange).toHaveBeenLastCalledWith("none");
		expect(onValueChange).toHaveBeenCalledTimes(1);
	});

	it("dismisses without pulling focus into a hidden pane and does not reopen when shown", async () => {
		stubMediaQueries({ belowMd: true });
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		const view = renderComponent(
			<Composer options={options} onValueChange={onValueChange} isPresented />,
		);

		const trigger = screen.getByRole("combobox", { name: "Select model" });
		await openPicker(user);
		await user.keyboard("gp");
		const focus = vi.spyOn(HTMLElement.prototype, "focus");

		view.rerender(
			<Composer
				options={options}
				onValueChange={onValueChange}
				isPresented={false}
			/>,
		);
		await act(async () => {
			await new Promise<void>((resolve) => setTimeout(resolve, 0));
		});
		expect(focus.mock.contexts).not.toContain(trigger);
		expect(trigger).not.toHaveFocus();

		view.rerender(
			<Composer options={options} onValueChange={onValueChange} isPresented />,
		);
		// Still closed after restore: this click opens rather than closes,
		// and the search typed before hiding is gone.
		const search = await openPicker(user);
		expect(search).toHaveFocus();
		expect(search).toHaveValue("");
		await user.keyboard("mini{Enter}");
		expect(onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	});

	it("hides one picker without disturbing another", async () => {
		stubMediaQueries({ belowMd: true });
		const user = userEvent.setup();
		const onFirstChange = vi.fn();
		const onSecondChange = vi.fn();
		const renderPickers = (firstPresented: boolean) => (
			<>
				<Composer
					options={options}
					onValueChange={onFirstChange}
					placeholder="First model"
					isPresented={firstPresented}
				/>
				<Composer
					options={options}
					onValueChange={onSecondChange}
					placeholder="Second model"
				/>
			</>
		);
		const view = renderComponent(renderPickers(true));

		await openPicker(user, "First model");
		await user.keyboard("gp");
		view.rerender(renderPickers(false));

		await openPicker(user, "Second model");
		await user.keyboard("mini{Enter}");
		expect(onSecondChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
		expect(onFirstChange).not.toHaveBeenCalled();

		view.rerender(renderPickers(true));
		expect(await openPicker(user, "First model")).toHaveValue("");
	});

	it.each([
		{ label: "above the md breakpoint", belowMd: false, docked: true },
		{ label: "without a composer to dock to", belowMd: true, docked: false },
	])("selects a model $label", async ({ belowMd, docked }) => {
		stubMediaQueries({ belowMd });
		const user = userEvent.setup();
		const onValueChange = vi.fn();
		renderComponent(
			<Composer
				options={options}
				onValueChange={onValueChange}
				docked={docked}
			/>,
		);

		await openPicker(user);
		await user.click(screen.getByRole("option", { name: /GPT-4o Mini/ }));

		expect(onValueChange).toHaveBeenCalledWith("openai/gpt-4o-mini");
	});

	it("measures only while open, never on the document root, and releases its listeners on close", async () => {
		stubMediaQueries({ belowMd: true });
		const viewport = stubVisualViewport();
		const user = userEvent.setup();
		renderComponent(<Composer options={options} onValueChange={vi.fn()} />);
		// Spies start after mount so React's own document listeners are not
		// counted.
		const viewportSubscribe = vi.spyOn(viewport, "addEventListener");
		const viewportUnsubscribe = vi.spyOn(viewport, "removeEventListener");
		const documentSubscribe = vi.spyOn(document, "addEventListener");
		const documentUnsubscribe = vi.spyOn(document, "removeEventListener");
		const rootWrite = vi.spyOn(document.documentElement.style, "setProperty");
		const styleWrite = vi.spyOn(CSSStyleDeclaration.prototype, "setProperty");

		await user.tab();
		expect(
			screen.getByRole("combobox", { name: "Select model" }),
		).toHaveFocus();
		expect(geometryWriteCount(styleWrite)).toBe(0);
		expect(listenersOf(documentSubscribe, "selectionchange")).toHaveLength(0);
		expect(viewportSubscribe).not.toHaveBeenCalled();

		await user.keyboard("{Enter}");
		await findSearch();
		expect(geometryWriteCount(styleWrite)).toBeGreaterThan(0);
		expect(rootWrite).not.toHaveBeenCalled();
		// Radix also listens to viewport scroll and resize; `selectionchange`
		// and `scrollend` are subscribed by the position hook alone.
		const [selectionChange] = listenersOf(documentSubscribe, "selectionchange");
		const [scrollEnd] = listenersOf(viewportSubscribe, "scrollend");
		expect(selectionChange).toBeDefined();
		expect(scrollEnd).toBeDefined();

		await user.keyboard("{Escape}");
		expect(documentUnsubscribe).toHaveBeenCalledWith(
			"selectionchange",
			selectionChange,
		);
		expect(viewportUnsubscribe).toHaveBeenCalledWith("scrollend", scrollEnd);
		expect(rootWrite).not.toHaveBeenCalled();
	});
});
