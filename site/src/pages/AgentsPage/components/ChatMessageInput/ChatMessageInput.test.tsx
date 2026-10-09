import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useLayoutEffect, useRef, useState } from "react";
import { type QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { skillsKey } from "#/api/queries/skills";
import type {
	AgentChatSendShortcut,
	SkillMetadata,
} from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { DEFAULT_AGENT_CHAT_SEND_SHORTCUT } from "../../utils/agentChatSendShortcut";
import { COMPACT_SLASH_COMMAND } from "../../utils/slashCommands";
import { ChatMessageInput, type ChatMessageInputRef } from "./ChatMessageInput";

const requiredProps = () => ({
	placeholder: "Type a message...",
	initialValue: "",
	onChange: vi.fn(),
	onEnter: vi.fn(),
	sendShortcut: DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
	disabled: false,
	hasWorkspace: false,
});

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
				{...requiredProps()}
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
				{...requiredProps()}
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
							{...requiredProps()}
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

	it("returns content inserted through the ref handle", async () => {
		const inputRef = { current: null as ChatMessageInputRef | null };
		renderWithQueryClient(
			<ChatMessageInput
				{...requiredProps()}
				ref={inputRef}
				aria-label="Chat message input"
			/>,
		);

		await waitFor(() => {
			expect(inputRef.current).not.toBeNull();
		});

		inputRef.current?.insertText("typed content");

		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("typed content");
		});
	});

	describe("slash menu skill sources", () => {
		const organizationId = "org-1";
		const mockReviewerSkill: SkillMetadata = {
			...MockSkill,
			id: "skill-reviewer",
			name: "reviewer",
		};
		const mockReleaseNotesSkill: SkillMetadata = {
			...MockSkill,
			id: "skill-release-notes",
			name: "release-notes",
		};
		const mockCompactSkill: SkillMetadata = {
			...MockSkill,
			id: "skill-compact",
			name: "compact",
		};
		const mockCompactorSkill: SkillMetadata = {
			...MockSkill,
			id: "skill-compactor",
			name: "compactor",
		};

		const renderWithSkills = ({
			personal,
			organization,
		}: {
			personal: SkillMetadata[];
			organization?: SkillMetadata[];
		}) => {
			const queryClient = createTestQueryClient();
			queryClient.setQueryData(
				skillsKey({ type: "user", user: "me" }),
				personal,
			);
			if (organization) {
				queryClient.setQueryData(
					skillsKey({ type: "organization", organizationId }),
					organization,
				);
			} else {
				vi.spyOn(API.experimental, "getOrganizationSkills").mockRejectedValue(
					new Error("Failed to load organization skills."),
				);
			}
			const inputRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					aria-label="Chat message input"
					organizationId={organizationId}
					slashCommands={[COMPACT_SLASH_COMMAND]}
				/>,
				queryClient,
			);
			return inputRef;
		};

		const pasteTrigger = async (text: string) => {
			const user = userEvent.setup();
			await user.click(
				screen.getByRole("textbox", { name: "Chat message input" }),
			);
			await user.paste(text);
			return user;
		};

		it("inserts the bare trigger of an organization skill", async () => {
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [mockReleaseNotesSkill],
			});
			const user = await pasteTrigger("/rel");
			await user.click(
				await screen.findByRole("option", { name: /release-notes/ }),
			);
			expect(inputRef.current?.getValue()).toBe("/release-notes");
		});

		it("qualifies a name shared by personal and organization skills", async () => {
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [mockReviewerSkill],
			});
			const user = await pasteTrigger("/rev");
			await user.click(
				await screen.findByRole("option", { name: /\/org\/reviewer/ }),
			);
			expect(inputRef.current?.getValue()).toBe("/org/reviewer");
		});

		it("ignores disabled skills when qualifying triggers", async () => {
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [{ ...mockReviewerSkill, enabled: false }],
			});
			const user = await pasteTrigger("/rev");
			await user.click(await screen.findByRole("option", { name: /reviewer/ }));
			expect(inputRef.current?.getValue()).toBe("/reviewer");
		});

		it("hides a built-in command an organization skill shadows", async () => {
			// With /compact hidden, Enter picks the first personal match.
			const inputRef = renderWithSkills({
				personal: [mockCompactorSkill],
				organization: [mockCompactSkill],
			});
			const user = await pasteTrigger("/comp");
			await screen.findByRole("option", { name: /compactor/ });
			await user.keyboard("{Enter}");
			expect(inputRef.current?.getValue()).toBe("/compactor");
		});

		it("offers built-in commands when the organization list fails", async () => {
			const inputRef = renderWithSkills({ personal: [mockCompactorSkill] });
			const user = await pasteTrigger("/comp");
			await screen.findByText(/Could not load organization skills/);
			await user.keyboard("{Enter}");
			expect(inputRef.current?.getValue()).toBe("/compact");
		});
	});
});
