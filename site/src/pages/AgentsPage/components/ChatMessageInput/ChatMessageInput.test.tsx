import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useLayoutEffect, useRef, useState } from "react";
import { type QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import { chatProjectsKey } from "#/api/queries/chatProjects";
import { organizationSkills } from "#/api/queries/organizationSkills";
import { userSkills } from "#/api/queries/userSkills";
import type {
	AgentChatSendShortcut,
	ChatMessagePart,
	ChatProject,
	ChatSkillSource,
	SkillMetadata,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { DEFAULT_AGENT_CHAT_SEND_SHORTCUT } from "../../utils/agentChatSendShortcut";
import { buildChatInputContent } from "../../utils/chatInputContent";
import { COMPACT_SLASH_COMMAND } from "../../utils/slashCommands";
import { getEditableContentPayload } from "../ChatConversation/messageParsing";
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

const mockSkillPart = (
	skill_source: ChatSkillSource,
	skill_name: string,
): ChatMessagePart => ({ type: "skill-invocation", skill_source, skill_name });

const getRequestContent = (
	inputRef: React.RefObject<ChatMessageInputRef | null>,
) =>
	buildChatInputContent({
		message: inputRef.current?.getValue() ?? "",
		composerParts: inputRef.current?.getContentParts() ?? [],
	}).content;

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
		const mockCompactorSkill: SkillMetadata = {
			...MockSkill,
			id: "skill-compactor",
			name: "compactor",
		};

		const renderWithSkills = ({
			personal,
			organization,
			projectId,
			projects = projectId ? [{ ...MockChatProject, id: projectId }] : [],
			staleProject,
			queryClient = createTestQueryClient(),
		}: {
			personal: SkillMetadata[];
			organization?: SkillMetadata[];
			projectId?: string;
			projects?: ChatProject[];
			// Project skills loaded long ago, so opening the menu refetches.
			staleProject?: SkillMetadata[];
			queryClient?: QueryClient;
		}) => {
			queryClient.setQueryData(userSkills().queryKey, personal);
			vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue(projects);
			if (projectId && staleProject) {
				queryClient.setQueryData(
					chatProjectSkillsKey(projectId),
					staleProject,
					{ updatedAt: 0 },
				);
			}
			if (organization) {
				queryClient.setQueryData(
					organizationSkills(organizationId).queryKey,
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
					projectId={projectId}
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

		it("sends an organization skill chosen with arrow keys as a chip between text", async () => {
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [
					{ ...MockSkill, id: "skill-release-notes", name: "release-notes" },
				],
			});
			const user = await pasteTrigger("Draft /");
			await screen.findByRole("option", { name: /^\/release-notes/ });
			await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");
			await user.keyboard("now");
			expect(getRequestContent(inputRef)).toEqual([
				{ type: "text", text: "Draft " },
				mockSkillPart("org", "release-notes"),
				{ type: "text", text: " now" },
			]);
		});

		it("ignores disabled skills when qualifying triggers", async () => {
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [{ ...mockReviewerSkill, enabled: false }],
			});
			const user = await pasteTrigger("/rev");
			await user.click(
				await screen.findByRole("option", { name: /^\/reviewer/ }),
			);
			expect(getRequestContent(inputRef)).toEqual([
				mockSkillPart("personal", "reviewer"),
			]);
		});

		it("inserts built-in commands as text", async () => {
			const inputRef = renderWithSkills({ personal: [], organization: [] });
			const user = await pasteTrigger("/comp");
			await user.click(
				await screen.findByRole("option", { name: /^\/compact/ }),
			);
			expect(getRequestContent(inputRef)).toEqual([
				{ type: "text", text: "/compact" },
			]);
		});

		it("hides built-in commands when the organization list fails", async () => {
			// With /compact hidden, Enter picks the personal match.
			const inputRef = renderWithSkills({
				personal: [mockCompactorSkill],
			});
			const user = await pasteTrigger("/comp");
			await screen.findByText(/Could not load organization skills/);
			await user.keyboard("{Enter}");
			expect(getRequestContent(inputRef)).toEqual([
				mockSkillPart("personal", "compactor"),
			]);
		});

		it.each([403, 404])(
			"offers built-in commands when the project list fails with %i",
			async (status) => {
				vi.spyOn(API.experimental, "getChatProjectSkills").mockRejectedValue({
					isAxiosError: true,
					response: { status, data: { message: "Project not found." } },
				});
				const queryClient = createTestQueryClient();
				const inputRef = renderWithSkills({
					personal: [mockCompactorSkill],
					organization: [],
					projectId: "project-1",
					queryClient,
				});
				const user = await pasteTrigger("/comp");
				await waitFor(() => {
					expect(
						queryClient.getQueryState(chatProjectSkillsKey("project-1"))
							?.status,
					).toBe("error");
				});
				await user.keyboard("{Enter}");
				expect(getRequestContent(inputRef)).toEqual([
					{ type: "text", text: "/compact" },
				]);
			},
		);

		it("drops loaded project skills when a refetch shows lost access", async () => {
			vi.spyOn(API.experimental, "getChatProjectSkills").mockRejectedValue({
				isAxiosError: true,
				response: { status: 404, data: { message: "Project not found." } },
			});
			const queryClient = createTestQueryClient();
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [],
				projectId: "project-1",
				staleProject: [mockReviewerSkill],
				queryClient,
			});
			const user = await pasteTrigger("/rev");
			await waitFor(() => {
				expect(
					queryClient.getQueryState(chatProjectSkillsKey("project-1"))?.status,
				).toBe("error");
			});
			await user.keyboard("{Enter}");
			// Without a project reviewer to collide with, Enter picks the
			// personal one.
			expect(getRequestContent(inputRef)).toEqual([
				mockSkillPart("personal", "reviewer"),
			]);
		});

		it("drops project skills when the caller's project list lacks the project", async () => {
			// An administrator role can read the skills of a project that is
			// not shared with the caller, but chatd does not load them.
			vi.spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue([
				mockReviewerSkill,
			]);
			const queryClient = createTestQueryClient();
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [],
				projectId: "project-1",
				projects: [],
				queryClient,
			});
			const user = await pasteTrigger("/rev");
			await waitFor(() => {
				expect(
					queryClient.getQueryState(chatProjectSkillsKey("project-1"))?.status,
				).toBe("success");
				expect(queryClient.getQueryState(chatProjectsKey)?.status).toBe(
					"success",
				);
			});
			await user.keyboard("{Enter}");
			expect(getRequestContent(inputRef)).toEqual([
				mockSkillPart("personal", "reviewer"),
			]);
		});

		it("hides built-in commands when the project list fails transiently", async () => {
			// Fake timers skip the delays of the query's retries.
			vi.useFakeTimers({ shouldAdvanceTime: true });
			try {
				const getChatProjectSkills = vi
					.spyOn(API.experimental, "getChatProjectSkills")
					.mockRejectedValue({
						isAxiosError: true,
						response: { status: 500, data: { message: "Internal error." } },
					});
				const inputRef = renderWithSkills({
					personal: [mockCompactorSkill],
					organization: [],
					projectId: "project-1",
				});
				const user = await pasteTrigger("/comp");
				await vi.advanceTimersByTimeAsync(10_000);
				await screen.findByText(/Could not load project skills/);
				expect(getChatProjectSkills).toHaveBeenCalledTimes(4);
				// With /compact hidden, Enter picks the personal match.
				await user.keyboard("{Enter}");
				expect(getRequestContent(inputRef)).toEqual([
					mockSkillPart("personal", "compactor"),
				]);
			} finally {
				vi.useRealTimers();
			}
		});

		it("keeps triggers qualified when the organization list fails", async () => {
			const inputRef = renderWithSkills({ personal: [mockReviewerSkill] });
			const user = await pasteTrigger("/rev");
			await screen.findByText(/Could not load organization skills/);
			await user.click(
				screen.getByRole("option", { name: /^\/personal\/reviewer/ }),
			);
			expect(getRequestContent(inputRef)).toEqual([
				mockSkillPart("personal", "reviewer"),
			]);
		});

		it("drops the slash trigger when the skill limit rejects a menu pick", async () => {
			const toastError = vi.spyOn(toast, "error");
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [],
			});
			await waitFor(() => expect(inputRef.current).not.toBeNull());
			act(() => {
				for (let index = 0; index < 8; index++) {
					inputRef.current?.insertSkill("project", `skill-${index}`);
				}
			});
			const user = await pasteTrigger("/rev");
			await user.click(await screen.findByRole("option", { name: /reviewer/ }));
			expect(
				getRequestContent(inputRef).filter(
					(part) => part.type === "skill-invocation",
				),
			).toHaveLength(8);
			expect(inputRef.current?.getValue()).not.toContain("/rev");
			expect(toastError).toHaveBeenCalledWith(
				"A message can include up to 8 skills.",
			);
		});
	});

	describe("insertSkill", () => {
		it("focuses the editor and inserts one chip per skill at the cursor", async () => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					aria-label="Chat message input"
				/>,
			);
			const textbox = screen.getByRole("textbox", {
				name: "Chat message input",
			});
			await user.click(textbox);
			await user.paste("Fix the flaky test");
			textbox.blur();

			inputRef.current?.insertSkill("project", "triage");
			inputRef.current?.insertSkill("project", "triage");
			inputRef.current?.insertSkill("personal", "triage");

			await waitFor(() => {
				expect(getRequestContent(inputRef)).toEqual([
					{ type: "text", text: "Fix the flaky test" },
					mockSkillPart("project", "triage"),
					mockSkillPart("personal", "triage"),
				]);
			});
			expect(textbox).toHaveFocus();
		});

		it("keeps typing at the caret after arrowing across a chip", async () => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					aria-label="Chat message input"
				/>,
			);
			await user.click(
				screen.getByRole("textbox", { name: "Chat message input" }),
			);
			await user.paste("Lead ");
			act(() => inputRef.current?.insertSkill("project", "triage"));
			await user.keyboard("tail{ArrowLeft>6/}X{ArrowRight}{ArrowRight}Y");

			await waitFor(() => {
				expect(getRequestContent(inputRef)).toEqual([
					{ type: "text", text: "Lead X" },
					mockSkillPart("project", "triage"),
					{ type: "text", text: " Ytail" },
				]);
			});
		});

		it("refuses a ninth skill chip and says why", async () => {
			const toastError = vi.spyOn(toast, "error");
			const inputRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					aria-label="Chat message input"
				/>,
			);
			await waitFor(() => expect(inputRef.current).not.toBeNull());
			act(() => {
				for (let index = 0; index < 9; index++) {
					inputRef.current?.insertSkill("project", `skill-${index}`);
				}
			});

			await waitFor(() =>
				expect(
					getRequestContent(inputRef).filter(
						(part) => part.type === "skill-invocation",
					),
				).toHaveLength(8),
			);
			expect(toastError).toHaveBeenCalledWith(
				"A message can include up to 8 skills.",
			);
		});

		it("removes a chip from the keyboard without sending the message", async () => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			const props = requiredProps();
			renderWithQueryClient(
				<ChatMessageInput
					{...props}
					ref={inputRef}
					aria-label="Chat message input"
				/>,
			);
			await waitFor(() => expect(inputRef.current).not.toBeNull());
			act(() => inputRef.current?.insertSkill("project", "triage"));
			await screen.findByRole("button", {
				name: "Remove skill /project/triage",
			});

			await user.tab();
			await user.keyboard("{Enter}");
			expect(getRequestContent(inputRef)).toEqual([]);
			expect(props.onEnter).not.toHaveBeenCalled();
			await waitFor(() =>
				expect(
					screen.getByRole("textbox", { name: "Chat message input" }),
				).toHaveFocus(),
			);
		});

		it("copies skill chips as aliases and pastes each available skill once", async () => {
			// Lexical's clipboard handlers check these jsdom-missing classes.
			vi.stubGlobal("ClipboardEvent", class ClipboardEvent extends Event {});
			vi.stubGlobal("DragEvent", class DragEvent extends MouseEvent {});
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					aria-label="Chat message input"
				/>,
			);
			await user.click(
				screen.getByRole("textbox", { name: "Chat message input" }),
			);
			await user.paste("Lead");
			// Without an organization, organization skills are unavailable.
			act(() => {
				inputRef.current?.insertSkill("personal", "triage");
				inputRef.current?.insertSkill("org", "release-notes");
			});
			await user.keyboard("tail{Control>}a{/Control}");
			const copied = await user.copy();
			expect(copied?.getData("text/plain")).toBe(
				"Lead/personal/triage /org/release-notes tail",
			);
			act(() => inputRef.current?.clear());
			await waitFor(() => {
				expect(getRequestContent(inputRef)).toEqual([]);
			});

			await user.paste(copied);
			await user.paste(copied);

			await waitFor(() => {
				expect(getRequestContent(inputRef)).toEqual([
					{ type: "text", text: "Lead" },
					mockSkillPart("personal", "triage"),
					{
						type: "text",
						text: " /org/release-notes tailLead /org/release-notes tail",
					},
				]);
			});

			const mimeType = "application/x-lexical-editor";
			const payload = JSON.parse(copied?.getData(mimeType) ?? "{}");
			copied?.setData(
				mimeType,
				JSON.stringify({ ...payload, nodes: [{ type: "unregistered" }] }),
			);
			act(() => inputRef.current?.clear());
			await user.paste(copied);
			await waitFor(() =>
				expect(getRequestContent(inputRef)).toEqual([
					{
						type: "text",
						text: "Lead/personal/triage /org/release-notes tail",
					},
				]),
			);
		});

		it("drops pasted skill chips beyond the limit and says why", async () => {
			vi.stubGlobal("ClipboardEvent", class ClipboardEvent extends Event {});
			vi.stubGlobal("DragEvent", class DragEvent extends MouseEvent {});
			const toastError = vi.spyOn(toast, "error");
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					aria-label="Chat message input"
				/>,
			);
			await user.click(
				screen.getByRole("textbox", { name: "Chat message input" }),
			);
			act(() => {
				for (let index = 0; index < 8; index++) {
					inputRef.current?.insertSkill("personal", `skill-${index}`);
				}
			});
			await user.keyboard("{Control>}a{/Control}");
			const copied = await user.copy();
			act(() => {
				inputRef.current?.clear();
				inputRef.current?.insertSkill("personal", "extra");
			});
			await user.paste(copied);

			await waitFor(() =>
				expect(
					getRequestContent(inputRef).filter(
						(part) => part.type === "skill-invocation",
					),
				).toHaveLength(8),
			);
			expect(toastError).toHaveBeenCalledWith(
				"A message can include up to 8 skills.",
			);
		});

		it("restores chips from a saved draft without React errors", async () => {
			const onChange = vi.fn();
			const inputRef = createRef<ChatMessageInputRef>();
			const { unmount } = renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={inputRef}
					onChange={onChange}
					aria-label="Chat message input"
				/>,
			);
			await waitFor(() => {
				expect(inputRef.current).not.toBeNull();
			});
			inputRef.current?.insertSkill("org", "release-notes");
			await waitFor(() => {
				expect(onChange).toHaveBeenLastCalledWith(
					expect.any(String),
					expect.any(String),
					true,
				);
			});
			const [, savedEditorState] = onChange.mock.lastCall ?? [];
			unmount();

			const errorSpy = vi.spyOn(console, "error");
			const restoredRef = createRef<ChatMessageInputRef>();
			renderWithQueryClient(
				<ChatMessageInput
					{...requiredProps()}
					ref={restoredRef}
					initialEditorState={savedEditorState}
					aria-label="Chat message input"
				/>,
			);
			await waitFor(() => {
				expect(getRequestContent(restoredRef)).toEqual([
					mockSkillPart("org", "release-notes"),
				]);
			});
			expect(errorSpy).not.toHaveBeenCalled();
		});

		const fileReferencePart: ChatMessagePart = {
			type: "file-reference",
			file_name: "main.go",
			start_line: 3,
			end_line: 5,
			content: "func main() {}",
		};

		it.each<[string, ChatMessagePart[]]>([
			[
				"skill and file-reference chips",
				[
					{ type: "text", text: "Use " },
					mockSkillPart("project", "triage"),
					{ type: "text", text: " on\nthe diff" },
					fileReferencePart,
					{ type: "text", text: " here" },
				],
			],
			[
				"file-reference chip",
				[{ type: "text", text: "Explain " }, fileReferencePart],
			],
		])(
			"reopens an edited message with its %s in place",
			async (_name, content) => {
				const { text, editorState } = getEditableContentPayload(content);
				const inputRef = createRef<ChatMessageInputRef>();
				renderWithQueryClient(
					<ChatMessageInput
						{...requiredProps()}
						ref={inputRef}
						initialValue={text}
						initialEditorState={editorState}
						aria-label="Chat message input"
					/>,
				);
				await waitFor(() => {
					expect(getRequestContent(inputRef)).toEqual(content);
				});
			},
		);
	});
});
