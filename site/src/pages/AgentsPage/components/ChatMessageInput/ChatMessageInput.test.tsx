import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import {
	type FC,
	type ReactNode,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { type QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { userSkills } from "#/api/queries/userSkills";
import { workspaceSkills } from "#/api/queries/workspaceSkills";
import type * as TypesGen from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatMessageInput, type ChatMessageInputRef } from "./ChatMessageInput";

const renderWithQueryClient = (
	children: ReactNode,
	queryClient: QueryClient = createTestQueryClient(),
) => {
	return render(
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>,
	);
};

const now = "2026-05-08T00:00:00Z";
const cachedPersonalSkills: TypesGen.UserSkillMetadata[] = [
	{
		id: "skill-cached-personal",
		name: "cached-personal",
		description: "Cached personal skill.",
		created_at: now,
		updated_at: now,
	},
];
const cachedWorkspaceSkills: TypesGen.WorkspaceSkillMetadata[] = [
	{
		name: "cached-workspace",
		description: "Cached workspace skill.",
	},
];

const pendingPromise = <T,>() =>
	new Promise<T>(() => {
		// Leave unresolved to model an in-flight background refetch.
	});

const InitialValueHarness: FC<{ initialValue: string }> = ({
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

const QueuedReplacementHarness: FC<{
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

	it("keeps cached skills selectable during background refetches", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(userSkills().queryKey, cachedPersonalSkills);
		queryClient.setQueryData(
			workspaceSkills("workspace-cached").queryKey,
			cachedWorkspaceSkills,
		);
		const getUserSkills = vi
			.spyOn(API.experimental, "getUserSkills")
			.mockReturnValue(pendingPromise<TypesGen.UserSkillMetadata[]>());
		const getWorkspaceSkills = vi
			.spyOn(API.experimental, "getWorkspaceSkills")
			.mockReturnValue(pendingPromise<TypesGen.WorkspaceSkillMetadata[]>());

		const inputRef = { current: null as ChatMessageInputRef | null };
		renderWithQueryClient(
			<ChatMessageInput
				ref={inputRef}
				workspaceId="workspace-cached"
				aria-label="Chat message input"
			/>,
			queryClient,
		);

		await waitFor(() => {
			expect(inputRef.current).not.toBeNull();
		});
		inputRef.current?.insertText("/");

		await waitFor(() => {
			expect(getUserSkills).toHaveBeenCalledWith("me");
			expect(getWorkspaceSkills).toHaveBeenCalledWith("workspace-cached");
		});
		const editor = await screen.findByTestId("chat-message-input");
		expect(await screen.findByText("/cached-personal")).toBeVisible();
		expect(await screen.findByText("/cached-workspace")).toBeVisible();

		fireEvent.keyDown(editor, { code: "Enter", key: "Enter" });

		await waitFor(() => {
			expect(editor.textContent).toBe("/cached-personal");
		});
	});

	it("keeps cached skills selectable after background refetch errors", async () => {
		const queryClient = createTestQueryClient();
		const personalQuery = userSkills();
		const workspaceQuery = workspaceSkills("workspace-cached");
		queryClient.setQueryData(personalQuery.queryKey, cachedPersonalSkills);
		queryClient.setQueryData(workspaceQuery.queryKey, cachedWorkspaceSkills);
		const personalError = new Error("personal skills failed");
		const workspaceError = new Error("workspace skills failed");
		const getUserSkills = vi
			.spyOn(API.experimental, "getUserSkills")
			.mockRejectedValue(personalError);
		const getWorkspaceSkills = vi
			.spyOn(API.experimental, "getWorkspaceSkills")
			.mockRejectedValue(workspaceError);

		const inputRef = { current: null as ChatMessageInputRef | null };
		renderWithQueryClient(
			<ChatMessageInput
				ref={inputRef}
				workspaceId="workspace-cached"
				aria-label="Chat message input"
			/>,
			queryClient,
		);

		await waitFor(() => {
			expect(inputRef.current).not.toBeNull();
		});
		inputRef.current?.insertText("/");

		await waitFor(() => {
			expect(getUserSkills).toHaveBeenCalledWith("me");
			expect(getWorkspaceSkills).toHaveBeenCalledWith("workspace-cached");
			expect(queryClient.getQueryState(personalQuery.queryKey)?.error).toBe(
				personalError,
			);
			expect(queryClient.getQueryState(workspaceQuery.queryKey)?.error).toBe(
				workspaceError,
			);
		});
		const editor = await screen.findByTestId("chat-message-input");
		expect(await screen.findByText("/cached-personal")).toBeVisible();
		expect(await screen.findByText("/cached-workspace")).toBeVisible();

		fireEvent.keyDown(editor, { code: "Enter", key: "Enter" });

		await waitFor(() => {
			expect(editor.textContent).toBe("/cached-personal");
		});
	});

	it("keeps cached empty skill lists authoritative after refetch errors", async () => {
		const queryClient = createTestQueryClient();
		const personalQuery = userSkills();
		const workspaceQuery = workspaceSkills("workspace-cached");
		queryClient.setQueryData(personalQuery.queryKey, []);
		queryClient.setQueryData(workspaceQuery.queryKey, []);
		const personalError = new Error("personal skills failed");
		const workspaceError = new Error("workspace skills failed");
		vi.spyOn(API.experimental, "getUserSkills").mockRejectedValue(
			personalError,
		);
		vi.spyOn(API.experimental, "getWorkspaceSkills").mockRejectedValue(
			workspaceError,
		);

		const inputRef = { current: null as ChatMessageInputRef | null };
		renderWithQueryClient(
			<ChatMessageInput
				ref={inputRef}
				workspaceId="workspace-cached"
				aria-label="Chat message input"
			/>,
			queryClient,
		);

		await waitFor(() => {
			expect(inputRef.current).not.toBeNull();
		});
		inputRef.current?.insertText("/");

		await waitFor(() => {
			expect(queryClient.getQueryState(personalQuery.queryKey)?.error).toBe(
				personalError,
			);
			expect(queryClient.getQueryState(workspaceQuery.queryKey)?.error).toBe(
				workspaceError,
			);
		});

		expect(
			await screen.findByText("No personal or workspace skills found."),
		).toBeVisible();
		expect(
			screen.queryByText(
				"Could not load personal skills. Close and type / again to retry.",
			),
		).not.toBeInTheDocument();
		expect(
			screen.queryByText(
				"Could not load workspace skills. Close and type / again to retry.",
			),
		).not.toBeInTheDocument();
	});
});
