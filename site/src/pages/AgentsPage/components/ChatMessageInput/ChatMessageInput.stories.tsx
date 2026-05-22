import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatMessageInput } from "./ChatMessageInput";

const now = "2026-05-08T00:00:00Z";

const mockSkills: TypesGen.UserSkillMetadata[] = [
	{
		id: "skill-reviewer",
		name: "reviewer",
		description: "Review changed files and suggest fixes.",
		created_at: now,
		updated_at: now,
	},
	{
		id: "skill-docs",
		name: "docs",
		description: "Draft docs for user-facing behavior.",
		created_at: now,
		updated_at: now,
	},
	{
		id: "skill-plan",
		name: "plan",
		description: "",
		created_at: now,
		updated_at: now,
	},
];

const meta: Meta<typeof ChatMessageInput> = {
	title: "components/ChatMessageInput/ChatMessageInput",
	component: ChatMessageInput,
	args: {
		"aria-label": "Chat message input",
		placeholder: "Message the agent",
		personalSkillsOverride: mockSkills,
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

const findVisibleText = async (text: string) => {
	let visibleElement: HTMLElement | undefined;
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		visibleElement = matches.find(
			(element) => element.getClientRects().length > 0,
		);
		expect(visibleElement).toBeDefined();
	});
	return visibleElement as HTMLElement;
};

const expectNoVisibleText = async (text: string) => {
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		expect(
			matches.every((element) => element.getClientRects().length === 0),
		).toBe(true);
	});
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
		expect(await findVisibleText("No personal skills found.")).toBeDefined();
		await userEvent.keyboard("{Enter}");
		expect(args.onEnter).not.toHaveBeenCalled();
		expect(editor.textContent).toBe("/");
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
		id: `skill-${index}`,
		name: `skill-${index}`,
		description: `Long description for skill ${index} that explains what it does in detail.`,
		created_at: now,
		updated_at: now,
	}),
);

// Decorator that:
// 1. Pins a fake composer to the bottom of the viewport so the popup
//    has a sensible anchor.
// 2. Sets `--mobile-dropdown-above-composer-bottom` and
//    `--mobile-dropdown-above-composer-max-height` directly on the
//    document element to simulate what `AgentChatInput` does in
//    production. We use values that place the popup just above the
//    fake composer and bound its height to the space above it.
// 3. Cleans up the CSS variables and the mocked matchMedia after the
//    story unmounts.
const MobileDecorator: Decorator = (Story) => {
	const composerHeight = 96; // matches mobile composer min-height
	const gap = 8;
	const aboveComposerBottom = composerHeight + gap;
	// Use the visual viewport height when available so the simulated
	// max-height matches what `AgentChatInput.tsx` computes in
	// production from `window.visualViewport`.
	const viewportHeight = window.visualViewport?.height ?? window.innerHeight;
	const aboveComposerMaxHeight = Math.max(
		0,
		viewportHeight - composerHeight - 16,
	);
	document.documentElement.style.setProperty(
		"--mobile-dropdown-above-composer-bottom",
		`${aboveComposerBottom}px`,
	);
	document.documentElement.style.setProperty(
		"--mobile-dropdown-above-composer-max-height",
		`${aboveComposerMaxHeight}px`,
	);
	return (
		<div
			data-testid="mobile-frame"
			style={{
				paddingBottom: composerHeight,
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
				<Story />
			</div>
		</div>
	);
};

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
			document.documentElement.style.removeProperty(
				"--mobile-dropdown-above-composer-bottom",
			);
			document.documentElement.style.removeProperty(
				"--mobile-dropdown-above-composer-max-height",
			);
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

			// At least one of the popup's scroll containers (wrapper or
			// inner list) must be scrollable, so the user can reach items
			// that overflow the available space.
			const scrollables: HTMLElement[] = [
				wrapper,
				...Array.from(wrapper.querySelectorAll<HTMLElement>("*")),
			];
			const hasScroll = scrollables.some(
				(node) => node.scrollHeight > node.clientHeight,
			);
			expect(hasScroll).toBe(true);
		} finally {
			restoreMatchMedia();
			document.documentElement.style.removeProperty(
				"--mobile-dropdown-above-composer-bottom",
			);
			document.documentElement.style.removeProperty(
				"--mobile-dropdown-above-composer-max-height",
			);
		}
	},
};
