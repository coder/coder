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
	ChatProject,
	SkillMetadata,
} from "#/api/typesGenerated";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { DEFAULT_AGENT_CHAT_SEND_SHORTCUT } from "../../utils/agentChatSendShortcut";
import { COMPACT_SLASH_COMMAND } from "../../utils/slashCommands";
import { ChatMessageInput, type ChatMessageInputRef } from "./ChatMessageInput";

// Isolate explicit focus requests from Lexical's automatic mount focus.
vi.mock("@lexical/react/LexicalAutoFocusPlugin", () => ({
	AutoFocusPlugin: () => null,
}));

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

const FocusBeforeReadyHarness: React.FC<{
	inputRef: React.RefObject<ChatMessageInputRef | null>;
	method: "focus" | "focusWhenEditable";
	disabled?: boolean;
	remountKey?: number;
}> = ({ inputRef, method, disabled = false, remountKey }) => {
	useLayoutEffect(() => {
		inputRef.current?.[method]();
	}, [inputRef, method]);

	return (
		<ChatMessageInput
			{...requiredProps()}
			ref={inputRef}
			initialValue="persisted draft"
			disabled={disabled}
			remountKey={remountKey}
			aria-label="Chat message input"
		/>
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

	it("ignores immediate focus until the editor is ready and editable", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const queryClient = createTestQueryClient();
		const harness = (disabled: boolean) => (
			<QueryClientProvider client={queryClient}>
				<FocusBeforeReadyHarness
					inputRef={inputRef}
					method="focus"
					disabled={disabled}
				/>
			</QueryClientProvider>
		);
		const { rerender } = render(harness(true));
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("persisted draft");
		});
		act(() => inputRef.current?.focus());

		await act(async () => rerender(harness(false)));
		await user.paste(" unsolicited");
		expect(inputRef.current?.getValue()).toBe("persisted draft");

		await act(async () => inputRef.current?.focus());
		await user.paste(" appended");
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("persisted draft appended");
		});
	});

	it("defers focus until editable across an inner remount and consumes the request once", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const queryClient = createTestQueryClient();
		const input = (disabled: boolean, remountKey = 0) => (
			<QueryClientProvider client={queryClient}>
				<button type="button">Focus elsewhere</button>
				<FocusBeforeReadyHarness
					inputRef={inputRef}
					method="focusWhenEditable"
					disabled={disabled}
					remountKey={remountKey}
				/>
			</QueryClientProvider>
		);
		const { rerender } = render(input(true));
		await user.click(
			screen.getByRole("textbox", { name: "Chat message input" }),
		);
		await user.keyboard("blocked{Enter}");
		expect(inputRef.current?.getValue()).toBe("persisted draft");

		await act(async () => rerender(input(true, 1)));
		await user.click(screen.getByRole("button", { name: "Focus elsewhere" }));
		await act(async () => rerender(input(false, 1)));
		await user.paste(" appended");
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("persisted draft appended");
		});

		await user.click(screen.getByRole("button", { name: "Focus elsewhere" }));
		await act(async () => rerender(input(true, 1)));
		await act(async () => rerender(input(false, 1)));
		await user.paste(" unsolicited");
		expect(inputRef.current?.getValue()).toBe("persisted draft appended");
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

		it("inserts the bare trigger of an organization skill chosen with arrow keys", async () => {
			const inputRef = renderWithSkills({
				personal: [mockReviewerSkill],
				organization: [
					{ ...MockSkill, id: "skill-release-notes", name: "release-notes" },
				],
			});
			const user = await pasteTrigger("/");
			await screen.findByRole("option", { name: /release-notes/ });
			await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");
			expect(inputRef.current?.getValue()).toBe("/release-notes");
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

		it("hides built-in commands when the organization list fails", async () => {
			// With /compact hidden, Enter picks the qualified personal match.
			const inputRef = renderWithSkills({
				personal: [mockCompactorSkill],
			});
			const user = await pasteTrigger("/comp");
			await screen.findByText(/Could not load organization skills/);
			await user.keyboard("{Enter}");
			expect(inputRef.current?.getValue()).toBe("/personal/compactor");
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
				expect(inputRef.current?.getValue()).toBe("/compact");
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
			// Without a project reviewer to collide with, the trigger stays bare.
			expect(inputRef.current?.getValue()).toBe("/reviewer");
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
			expect(inputRef.current?.getValue()).toBe("/reviewer");
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
				// With /compact hidden, Enter picks the qualified personal match.
				await user.keyboard("{Enter}");
				expect(inputRef.current?.getValue()).toBe("/personal/compactor");
			} finally {
				vi.useRealTimers();
			}
		});

		it("keeps triggers qualified when the organization list fails", async () => {
			const inputRef = renderWithSkills({ personal: [mockReviewerSkill] });
			const user = await pasteTrigger("/rev");
			await screen.findByText(/Could not load organization skills/);
			await user.click(screen.getByRole("option", { name: /reviewer/ }));
			expect(inputRef.current?.getValue()).toBe("/personal/reviewer");
		});
	});
});
