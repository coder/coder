import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act, createRef, useLayoutEffect, useRef, useState } from "react";
import { type QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { AgentChatSendShortcut } from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatPresentationContext } from "../ChatPresentationContext";
import { ChatMessageInput, type ChatMessageInputRef } from "./ChatMessageInput";
import { MockSkill } from "./storyHelpers";

const renderWithQueryClient = (
	children: React.ReactNode,
	queryClient: QueryClient = createTestQueryClient(),
) => {
	return render(
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
	);
};

const InitialValueHarness: React.FC<{ initialValue: string }> = ({
	initialValue,
}) => {
	const inputRef = useRef<ChatMessageInputRef>(null);
	const [observedValue, setObservedValue] = useState("");

	useLayoutEffect(() => {
		setObservedValue(inputRef.current?.getValue() ?? "");
	}, []);

	return (
		<>
			<div data-testid="observed-value">{observedValue}</div>
			<ChatMessageInput
				ref={inputRef}
				initialValue={initialValue}
				aria-label="Chat message input"
			/>
		</>
	);
};

const QueuedReplacementHarness: React.FC<{
	initialValue: string;
	replacementValue: string;
}> = ({ initialValue, replacementValue }) => {
	const inputRef = useRef<ChatMessageInputRef>(null);
	const [observedValue, setObservedValue] = useState("");

	useLayoutEffect(() => {
		inputRef.current?.setValue(replacementValue);
		setObservedValue(inputRef.current?.getValue() ?? "");
	}, [replacementValue]);

	return (
		<>
			<div data-testid="observed-value">{observedValue}</div>
			<ChatMessageInput
				ref={inputRef}
				initialValue={initialValue}
				aria-label="Chat message input"
			/>
		</>
	);
};

beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

describe("ChatMessageInput", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.unstubAllGlobals();
	});

	describe.each([390, 640, 1280])("at %ipx wide", (width) => {
		describe.each([false, true])("with coarse pointer %s", (coarsePointer) => {
			it.each<{
				shortcut: AgentChatSendShortcut;
				keys: string;
				modifier: boolean;
				shift: boolean;
			}>([
				{ shortcut: "enter", keys: "{Enter}", modifier: false, shift: false },
				{
					shortcut: "enter",
					keys: "{Meta>}{Enter}{/Meta}",
					modifier: true,
					shift: false,
				},
				{
					shortcut: "enter",
					keys: "{Control>}{Enter}{/Control}",
					modifier: true,
					shift: false,
				},
				{
					shortcut: "enter",
					keys: "{Shift>}{Enter}{/Shift}",
					modifier: false,
					shift: true,
				},
				{
					shortcut: "modifier_enter",
					keys: "{Enter}",
					modifier: false,
					shift: false,
				},
				{
					shortcut: "modifier_enter",
					keys: "{Meta>}{Enter}{/Meta}",
					modifier: true,
					shift: false,
				},
				{
					shortcut: "modifier_enter",
					keys: "{Control>}{Enter}{/Control}",
					modifier: true,
					shift: false,
				},
				{
					shortcut: "modifier_enter",
					keys: "{Shift>}{Meta>}{Enter}{/Meta}{/Shift}",
					modifier: true,
					shift: true,
				},
			])(
				"handles $keys with the $shortcut preference",
				async ({ shortcut, keys, modifier, shift }) => {
					vi.stubGlobal(
						"matchMedia",
						(query: string): MediaQueryList => ({
							matches:
								query === "(pointer: coarse)"
									? coarsePointer
									: query === "(max-width: 639px)" && width < 640,
							media: query,
							onchange: null,
							addListener: vi.fn(),
							removeListener: vi.fn(),
							addEventListener: vi.fn(),
							removeEventListener: vi.fn(),
							dispatchEvent: vi.fn(),
						}),
					);
					const user = userEvent.setup();
					const inputRef = createRef<ChatMessageInputRef>();
					const onEnter = vi.fn();
					renderWithQueryClient(
						<ChatMessageInput
							ref={inputRef}
							aria-label="Chat message input"
							sendShortcut={shortcut}
							onEnter={onEnter}
						/>,
					);
					await user.click(
						screen.getByRole("textbox", { name: "Chat message input" }),
					);
					await user.paste("Draft");
					await user.keyboard(keys);

					const shouldSend =
						!shift && (modifier || (!coarsePointer && shortcut === "enter"));
					await waitFor(() => {
						expect(inputRef.current?.getValue()).toBe(
							shouldSend ? "Draft" : "Draft\n",
						);
					});
					expect(onEnter).toHaveBeenCalledTimes(shouldSend ? 1 : 0);
				},
			);
		});
	});

	it("returns the initial draft before the editor visually hydrates", async () => {
		renderWithQueryClient(
			<InitialValueHarness initialValue="persisted draft" />,
		);

		expect(screen.getByTestId("observed-value")).toHaveTextContent(
			"persisted draft",
		);
		await waitFor(() => {
			expect(screen.getByTestId("chat-message-input").textContent).toBe(
				"persisted draft",
			);
		});
	});

	it("queues setValue calls made before the editor is ready", async () => {
		renderWithQueryClient(
			<QueuedReplacementHarness
				initialValue="persisted draft"
				replacementValue="queued replacement"
			/>,
		);

		expect(screen.getByTestId("observed-value")).toHaveTextContent(
			"queued replacement",
		);
		await waitFor(() => {
			expect(screen.getByTestId("chat-message-input").textContent).toBe(
				"queued replacement",
			);
		});
	});

	it("dismisses slash completion on hide while preserving the editor and its draft", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onEnter = vi.fn();
		const input = (isPresented: boolean) => (
			<ChatPresentationContext value={isPresented}>
				<button type="button">Outside editor</button>
				<ChatMessageInput
					ref={inputRef}
					aria-label="Chat message input"
					personalSkillsOverride={[MockSkill]}
					onEnter={onEnter}
				/>
			</ChatPresentationContext>
		);
		const queryClient = createTestQueryClient();
		const rendered = renderWithQueryClient(input(true), queryClient);
		const editor = screen.getByRole("textbox", { name: "Chat message input" });
		await user.click(editor);
		await user.paste("/");
		await waitFor(() => expect(inputRef.current?.getValue()).toBe("/"));
		const focus = vi.spyOn(editor, "focus");
		try {
			rendered.rerender(
				<QueryClientProvider client={queryClient}>
					{input(false)}
				</QueryClientProvider>,
			);
			await user.click(screen.getByRole("button", { name: "Outside editor" }));
			await act(async () => {
				inputRef.current?.focus();
			});
			expect(focus).not.toHaveBeenCalled();
			expect(inputRef.current?.getValue()).toBe("/");
			rendered.rerender(
				<QueryClientProvider client={queryClient}>
					{input(true)}
				</QueryClientProvider>,
			);
			await user.click(editor);
			await user.keyboard("{Enter}");
			expect(onEnter).toHaveBeenCalledOnce();
			expect(inputRef.current?.getValue()).toBe("/");
		} finally {
			focus.mockRestore();
		}
	});

	it.each([false, true])(
		"decides autofocus per editor mount, not on pane restore (initially presented: %s)",
		async (initiallyPresented) => {
			const queryClient = createTestQueryClient();
			const focus = vi.spyOn(HTMLElement.prototype, "focus");
			const input = (isPresented: boolean, remountKey: number) => (
				<QueryClientProvider client={queryClient}>
					<ChatPresentationContext value={isPresented}>
						<ChatMessageInput
							aria-label="Chat message input"
							personalSkillsOverride={[]}
							autoFocus
							remountKey={remountKey}
						/>
					</ChatPresentationContext>
				</QueryClientProvider>
			);
			const view = render(input(initiallyPresented, 0));
			await act(async () => {});
			focus.mockClear();
			view.rerender(input(false, 1));
			await act(async () => {});
			const hiddenEditor = screen.getByRole("textbox", {
				name: "Chat message input",
			});
			expect(focus.mock.contexts).not.toContain(hiddenEditor);
			focus.mockClear();
			view.rerender(input(true, 1));
			await act(async () => {});
			expect(focus.mock.contexts).not.toContain(hiddenEditor);
			view.rerender(input(true, 2));
			const editingEditor = screen.getByRole("textbox", {
				name: "Chat message input",
			});
			await waitFor(() => expect(focus.mock.contexts).toContain(editingEditor));
		},
	);

	it("returns updated content even without an external onChange prop", async () => {
		const inputRef = { current: null as ChatMessageInputRef | null };
		renderWithQueryClient(
			<ChatMessageInput ref={inputRef} aria-label="Chat message input" />,
		);

		await waitFor(() => {
			expect(inputRef.current).not.toBeNull();
		});

		inputRef.current?.insertText("typed content");

		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("typed content");
		});
	});
});
