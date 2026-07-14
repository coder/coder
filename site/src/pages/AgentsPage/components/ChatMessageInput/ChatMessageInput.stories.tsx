import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { type PropsWithChildren, useEffect } from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatMessageInput } from "./ChatMessageInput";
import {
	expectNoVisibleText,
	findVisibleText,
	MockSkill,
	MockSkills,
} from "./storyHelpers";

const meta: Meta<typeof ChatMessageInput> = {
	title: "components/ChatMessageInput/ChatMessageInput",
	component: ChatMessageInput,
	args: {
		"aria-label": "Chat message input",
		placeholder: "Message the agent",
		personalSkillsOverride: MockSkills,
		onChange: fn(),
		onEnter: fn(),
	},
	decorators: [
		(Story) => (
			<div className="w-[520px] space-y-3 rounded-md border border-border border-solid p-4">
				<button type="button" className="text-content-secondary text-sm">
					Outside target
				</button>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ChatMessageInput>;

const expectNoVisibleTextImmediately = (text: string) => {
	const matches = within(document.body).queryAllByText(text);
	expect(
		matches.every((element) => element.getClientRects().length === 0),
	).toBe(true);
};

const editorFromCanvas = (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	return canvas.getByTestId("chat-message-input");
};

const typeInEditor = async (canvasElement: HTMLElement, text: string) => {
	const editor = editorFromCanvas(canvasElement);
	await userEvent.click(editor);
	await userEvent.keyboard(text);
	return editor;
};

export const Closed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("chat-message-input")).toBeVisible();
		await expectNoVisibleText("/reviewer");
	},
};

export const OpensWithSkills: Story = {
	play: async ({ canvasElement }) => {
		await typeInEditor(canvasElement, "/");
		expect(await findVisibleText("/reviewer")).toBeDefined();
		expect(
			await findVisibleText("Review changed files and suggest fixes."),
		).toBeDefined();
	},
};

export const EmptySkills: Story = {
	args: {
		personalSkillsOverride: [],
		onEnter: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const editor = await typeInEditor(canvasElement, "/");
		// "/" is plain text when the skills list is empty.
		expectNoVisibleTextImmediately("No personal skills found.");
		await userEvent.keyboard("{Enter}");
		expect(args.onEnter).toHaveBeenCalledTimes(1);
		expect(editor.textContent).toBe("/");
	},
};

export const FilteredEmptyKeepsMenuOpen: Story = {
	args: {
		onEnter: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const editor = await typeInEditor(canvasElement, "/zzzz");
		expect(
			await findVisibleText("No personal skills match that query."),
		).toBeDefined();
		await userEvent.keyboard("{Enter}");
		expect(args.onEnter).not.toHaveBeenCalled();
		expect(editor.textContent).toBe("/zzzz");
	},
};

export const FiltersByQuery: Story = {
	play: async ({ canvasElement }) => {
		await typeInEditor(canvasElement, "/rev");
		expect(await findVisibleText("/reviewer")).toBeDefined();
		await expectNoVisibleText("/docs");
	},
};

export const EnterSelectsSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/rev");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Enter}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/reviewer");
		});
		await expectNoVisibleText("Review changed files and suggest fixes.");
	},
};

export const ArrowKeysSelectHighlightedSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/docs");
		await userEvent.keyboard("{ArrowDown}{Enter}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/plan");
		});
	},
};

export const TabSelectsSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/rev");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Tab}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/reviewer");
		});
		await expectNoVisibleText("Review changed files and suggest fixes.");
	},
};

export const ClickSelectsSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/rev");
		await userEvent.click(await findVisibleText("/reviewer"));
		await waitFor(() => {
			expect(editor.textContent).toBe("/reviewer");
		});
		await expectNoVisibleText("Review changed files and suggest fixes.");
	},
};

export const EmptyDescriptionInsertsNameOnly: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/pla");
		await findVisibleText("/plan");
		await userEvent.keyboard("{Enter}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/plan");
		});
	},
};

export const SlashInsideUrlDoesNotOpen: Story = {
	play: async ({ canvasElement }) => {
		await typeInEditor(canvasElement, "https://");
		await expectNoVisibleText("/reviewer");
	},
};

export const BackspaceClosesWithoutEmptyStateFlash: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Backspace}");

		expect(editor.textContent).toBe("");
		expectNoVisibleTextImmediately("No personal skills found.");
		await expectNoVisibleText("/reviewer");
	},
};

export const EscapeClosesWithoutReplacing: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Escape}");
		await expectNoVisibleText("/reviewer");
		await waitFor(() => {
			expect(editor).toHaveFocus();
		});
		expect(editor.textContent).toBe("/");
		await userEvent.keyboard("r");
		await expectNoVisibleText("/reviewer");
		expect(editor.textContent).toBe("/r");
	},
};

export const OutsideClickClosesWithoutReplacing: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/reviewer");
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Outside target" }),
		);
		await expectNoVisibleText("/reviewer");
		expect(editor.textContent).toBe("/");
	},
};

export const OutsideClickDismissesTriggerOnRefocus: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/reviewer");
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Outside target" }),
		);
		await expectNoVisibleText("/reviewer");
		await userEvent.click(editor);
		await expectNoVisibleText("/reviewer");
		expect(editor.textContent).toBe("/");
	},
};

// Stories below verify that on mobile viewports, the personal skills
// popup sits directly above the chat input rather than being clipped
// above the visible viewport.

const MOBILE_MEDIA_QUERY = "(max-width: 767px)";

// Mock window.matchMedia so the `.mobile-full-width-dropdown*` CSS
// branches in `site/src/index.css` activate even when Storybook's
// outer viewport differs from the simulated mobile width.
const mockMobileMatchMedia = (): (() => void) => {
	const originalMatchMedia = window.matchMedia;
	window.matchMedia = (query: string) =>
		({
			matches: query === MOBILE_MEDIA_QUERY,
			media: query,
			onchange: null,
			addEventListener: () => undefined,
			removeEventListener: () => undefined,
			dispatchEvent: () => true,
			addListener: () => undefined,
			removeListener: () => undefined,
		}) as MediaQueryList;
	return () => {
		window.matchMedia = originalMatchMedia;
	};
};

const longSkillList: TypesGen.UserSkillMetadata[] = Array.from(
	{ length: 30 },
	(_, index) => ({
		...MockSkill,
		id: `skill-${index}`,
		name: `skill-${index}`,
		description: `Long description for skill ${index} that explains what it does in detail.`,
	}),
);

const mobileDropdownProperties = [
	"--mobile-dropdown-left",
	"--mobile-dropdown-width",
	"--mobile-dropdown-above-composer-bottom",
	"--mobile-dropdown-above-composer-max-height",
] as const;

const MOBILE_COMPOSER_HEIGHT = 96; // matches mobile composer min-height
const MOBILE_COMPOSER_GAP = 8;
const MOBILE_VIEWPORT_PADDING = 16;
const MOBILE_MINIMUM_MENU_HEIGHT = 96;

const setMobileDropdownGeometry = (options?: {
	visualViewportOffsetTop?: number;
}) => {
	const composerTop = innerHeight - MOBILE_COMPOSER_HEIGHT;
	const visualViewportOffsetTop = options?.visualViewportOffsetTop ?? 0;
	document.documentElement.style.setProperty("--mobile-dropdown-left", "1rem");
	document.documentElement.style.setProperty(
		"--mobile-dropdown-width",
		"calc(100vw - 2rem)",
	);
	document.documentElement.style.setProperty(
		"--mobile-dropdown-above-composer-bottom",
		`${innerHeight - composerTop + MOBILE_COMPOSER_GAP}px`,
	);
	const maxHeightCandidates = [
		composerTop -
			visualViewportOffsetTop -
			MOBILE_COMPOSER_GAP -
			MOBILE_VIEWPORT_PADDING,
		composerTop - MOBILE_COMPOSER_GAP - MOBILE_VIEWPORT_PADDING,
	].filter((height) => height > 0);
	const maxHeight = Math.max(
		MOBILE_MINIMUM_MENU_HEIGHT,
		maxHeightCandidates.length > 0 ? Math.min(...maxHeightCandidates) : 0,
	);
	document.documentElement.style.setProperty(
		"--mobile-dropdown-above-composer-max-height",
		`${maxHeight}px`,
	);

	return { composerTop, maxHeight, visualViewportOffsetTop };
};

const clearMobileDropdownGeometry = () => {
	for (const property of mobileDropdownProperties) {
		document.documentElement.style.removeProperty(property);
	}
};

const MobileFrame = ({ children }: PropsWithChildren) => {
	useEffect(() => {
		setMobileDropdownGeometry();
		return clearMobileDropdownGeometry;
	}, []);

	return (
		<div
			data-testid="mobile-frame"
			style={{
				paddingBottom: MOBILE_COMPOSER_HEIGHT,
				height: "100vh",
				position: "relative",
			}}
		>
			<button type="button" className="text-content-secondary text-sm">
				Outside target
			</button>
			<div
				style={{
					position: "fixed",
					bottom: 0,
					left: "1rem",
					width: "calc(100vw - 2rem)",
				}}
			>
				{children}
			</div>
		</div>
	);
};

// Decorator that pins a fake composer to the bottom of the viewport and sets
// mobile dropdown geometry custom properties to simulate `AgentChatInput`.
const MobileDecorator: Decorator = (Story) => (
	<MobileFrame>
		<Story />
	</MobileFrame>
);

// Verifies the popup wrapper is positioned above the chat input on
// mobile: position: fixed, full chat-input width, bottom edge at the
// CSS variable, and top edge inside the visible viewport.
export const MobileAboveChatInput: Story = {
	decorators: [MobileDecorator],
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		chromatic: { viewports: [320] },
	},
	play: async ({ canvasElement }) => {
		const restoreMatchMedia = mockMobileMatchMedia();
		try {
			await typeInEditor(canvasElement, "/");
			const skillItem = await findVisibleText("/reviewer");
			// Walk up to the radix popper wrapper that the CSS targets.
			const wrapper = skillItem.closest(
				"[data-radix-popper-content-wrapper]",
			) as HTMLElement | null;
			expect(wrapper).not.toBeNull();
			if (!wrapper) return;

			const rect = wrapper.getBoundingClientRect();
			const styles = window.getComputedStyle(wrapper);
			expect(styles.position).toBe("fixed");
			// Popup must stay fully inside the visible viewport, with its
			// bottom edge above the simulated chat input.
			expect(rect.top).toBeGreaterThanOrEqual(0);
			expect(rect.bottom).toBeLessThanOrEqual(window.innerHeight);
		} finally {
			restoreMatchMedia();
		}
	},
};

// Verifies that the popup remains inside a panned visual viewport,
// which is what iOS WebKit browsers do when the soft keyboard opens.
export const MobileShiftedVisualViewport: Story = {
	decorators: [MobileDecorator],
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { exclude: true },
	},
	play: async ({ canvasElement }) => {
		const restoreMatchMedia = mockMobileMatchMedia();
		const { composerTop, visualViewportOffsetTop } = setMobileDropdownGeometry({
			visualViewportOffsetTop: Math.min(
				160,
				Math.max(0, innerHeight - MOBILE_COMPOSER_HEIGHT - 80),
			),
		});

		try {
			await typeInEditor(canvasElement, "/");
			const skillItem = await findVisibleText("/reviewer");
			const wrapper = skillItem.closest(
				"[data-radix-popper-content-wrapper]",
			) as HTMLElement | null;
			expect(wrapper).not.toBeNull();
			if (!wrapper) return;

			const rect = wrapper.getBoundingClientRect();
			expect(rect.top).toBeGreaterThanOrEqual(visualViewportOffsetTop);
			expect(rect.bottom).toBeLessThanOrEqual(
				composerTop - MOBILE_COMPOSER_GAP,
			);
		} finally {
			restoreMatchMedia();
		}
	},
};

// Verifies an over-large visual viewport offset does not collapse the menu.
export const MobileOffsetTopDoesNotCollapse: Story = {
	decorators: [MobileDecorator],
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { exclude: true },
	},
	play: async ({ canvasElement }) => {
		const restoreMatchMedia = mockMobileMatchMedia();
		const { maxHeight } = setMobileDropdownGeometry({
			visualViewportOffsetTop: innerHeight,
		});

		try {
			await typeInEditor(canvasElement, "/");
			const skillItem = await findVisibleText("/reviewer");
			const wrapper = skillItem.closest(
				"[data-radix-popper-content-wrapper]",
			) as HTMLElement | null;
			expect(wrapper).not.toBeNull();
			if (!wrapper) return;

			expect(maxHeight).toBeGreaterThanOrEqual(MOBILE_MINIMUM_MENU_HEIGHT);
			expect(Number.parseFloat(getComputedStyle(wrapper).maxHeight)).toBe(
				maxHeight,
			);
			expect(wrapper.getBoundingClientRect().height).toBeGreaterThan(0);
		} finally {
			restoreMatchMedia();
		}
	},
};

// Verifies the popup scrolls internally when the skills list is
// taller than the available space above the chat input. The wrapper
// height must stay bounded by the CSS max-height, and at least one
// scroll container inside must be scrollable.
export const MobileLongListScrolls: Story = {
	args: {
		personalSkillsOverride: longSkillList,
	},
	decorators: [MobileDecorator],
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		chromatic: { viewports: [320] },
	},
	play: async ({ canvasElement }) => {
		const restoreMatchMedia = mockMobileMatchMedia();
		setMobileDropdownGeometry({
			visualViewportOffsetTop: Math.max(
				0,
				innerHeight -
					MOBILE_COMPOSER_HEIGHT -
					MOBILE_COMPOSER_GAP -
					MOBILE_VIEWPORT_PADDING -
					MOBILE_MINIMUM_MENU_HEIGHT,
			),
		});

		try {
			await typeInEditor(canvasElement, "/");
			const skillItem = await findVisibleText("/skill-0");
			const wrapper = skillItem.closest(
				"[data-radix-popper-content-wrapper]",
			) as HTMLElement | null;
			expect(wrapper).not.toBeNull();
			if (!wrapper) return;

			const rect = wrapper.getBoundingClientRect();
			expect(rect.top).toBeGreaterThanOrEqual(0);
			expect(rect.bottom).toBeLessThanOrEqual(window.innerHeight);

			const commandList = skillItem.closest(
				"[cmdk-list]",
			) as HTMLElement | null;
			expect(commandList).not.toBeNull();
			if (!commandList) return;

			const hasVisibleVerticalScrollbar = (node: HTMLElement) => {
				const overflowY = getComputedStyle(node).overflowY;
				return (
					(overflowY === "auto" || overflowY === "scroll") &&
					node.scrollHeight > node.clientHeight
				);
			};
			const scrollableNodes = [
				wrapper,
				...Array.from(wrapper.querySelectorAll<HTMLElement>("*")),
			].filter(hasVisibleVerticalScrollbar);

			expect(commandList.scrollHeight).toBeGreaterThan(
				commandList.clientHeight,
			);
			expect(scrollableNodes).toHaveLength(1);
			expect(scrollableNodes[0]).toBe(commandList);
		} finally {
			restoreMatchMedia();
		}
	},
};
