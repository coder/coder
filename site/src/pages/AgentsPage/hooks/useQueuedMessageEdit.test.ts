import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, useState } from "react";
import { QueryClient, QueryClientProvider, useMutation } from "react-query";
import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatQueuedMessage,
	MockChatQueuedMessageUnderEdit,
} from "#/testHelpers/chatEntities";
import { createChatStore } from "../components/ChatConversation/chatStore";
import {
	type ComposerMode,
	deriveComposerTarget,
	useQueuedMessageEdit,
} from "./useQueuedMessageEdit";

const mockRow5 = { ...MockChatQueuedMessage, id: 5 };
const mockRow5Marked = { ...MockChatQueuedMessageUnderEdit, id: 5 };
const mockRow6 = { ...MockChatQueuedMessage, id: 6 };
const mockRow6Marked = { ...MockChatQueuedMessageUnderEdit, id: 6 };

const queued = (id: number) => ({ kind: "queued", id }) as const;
const history = (id: number) => ({ kind: "history", id }) as const;

describe("the row the composer edits", () => {
	const owner = true;
	const viewer = false;
	const cases: Array<[string, ComposerMode, number | null, boolean, unknown]> =
		[
			[
				"an untouched composer follows the marked row",
				"follow",
				5,
				owner,
				queued(5),
			],
			[
				"an untouched composer with no marked row edits nothing",
				"follow",
				null,
				owner,
				null,
			],
			[
				"a viewer's untouched composer never follows the marked row",
				"follow",
				5,
				viewer,
				null,
			],
			["a draft ignores the marked row", "draft", 5, owner, null],
			["a history choice stays in effect", history(3), 5, owner, history(3)],
			[
				"a queued choice stays in effect while that row is marked",
				queued(5),
				5,
				owner,
				queued(5),
			],
			[
				"a queued choice closes once another row is marked",
				queued(5),
				6,
				owner,
				null,
			],
		];
	it.each(cases)("%s", (_name, composerMode, markedID, isOwner, expected) => {
		expect(deriveComposerTarget(composerMode, markedID, isOwner)).toEqual(
			expected,
		);
	});
});

type Patch = {
	id: number;
	req: TypesGen.EditChatQueuedMessageRequest;
	resolve: () => void;
	reject: (error: Error) => void;
};

// As on the page, the store shows the begin or end at the click, and the
// request is a real mutation in the queue scope, so a later request waits
// for an earlier one.
const setup = (options?: {
	storeQueue?: readonly TypesGen.ChatQueuedMessage[];
	composerMode?: ComposerMode;
}) => {
	const store = createChatStore();
	store.seed({
		messages: [],
		queued_messages: options?.storeQueue ?? [],
		has_more: false,
		snapshot_version: 1,
	});
	const queryClient = new QueryClient();
	const patches: Patch[] = [];
	const hook = renderHook(
		() => {
			const [composerMode, setComposerMode] = useState<ComposerMode>(
				options?.composerMode ?? "follow",
			);
			const marker = useMutation({
				mutationFn: ({
					queuedMessageId,
					req,
				}: {
					queuedMessageId: number;
					req: TypesGen.EditChatQueuedMessageRequest;
				}) =>
					new Promise<void>((resolve, reject) => {
						patches.push({ id: queuedMessageId, req, resolve, reject });
					}),
				scope: { id: "chat-queued-messages-test" },
			});
			const edit = useQueuedMessageEdit({
				store,
				composerMode,
				setComposerMode,
				isOwner: true,
				setQueuedMessageEditing: (id, editing) =>
					store.runPendingAction(
						{ kind: "editQueued", queuedMessageID: id, editing },
						() => marker.mutateAsync({ queuedMessageId: id, req: { editing } }),
					),
			});
			return { ...edit, composerMode, markerStatus: marker.status };
		},
		{
			wrapper: ({ children }: { children: React.ReactNode }) =>
				createElement(QueryClientProvider, { client: queryClient }, children),
		},
	);
	const result = () => hook.result.current;
	const beginEdit = (id: number) =>
		act(() => result().handleEditQueuedMessage(id));
	const settle = (patch: Patch, outcome: "resolve" | "reject") =>
		act(() => {
			if (outcome === "resolve") {
				patch.resolve();
			} else {
				patch.reject(new Error("request failed"));
			}
		});
	return { store, patches, result, beginEdit, settle };
};

describe("useQueuedMessageEdit", () => {
	it("on reload, targets the marked row as soon as the store holds it and leaves the composer untouched", () => {
		const t = setup();
		expect(t.result().composerTarget).toBeNull();

		act(() => t.store.setQueuedMessages([mockRow5Marked]));
		expect(t.result().composerTarget).toEqual(queued(5));
		expect(t.result().composerMode).toBe("follow");
	});

	it("Edit opens the composer and shows the row as under edit before the begin request settles", async () => {
		const t = setup({ storeQueue: [mockRow5] });

		t.beginEdit(5);
		expect(t.result().markedID).toBe(5);
		expect(t.result().composerTarget).toEqual(queued(5));
		await waitFor(() => expect(t.patches).toHaveLength(1));
		expect(t.patches.map(({ id, req }) => ({ id, req }))).toEqual([
			{ id: 5, req: { editing: true } },
		]);

		act(() => t.store.setQueuedMessages([mockRow5Marked]));
		await t.settle(t.patches[0], "resolve");
		await waitFor(() => expect(t.result().markerStatus).toBe("success"));
		expect(t.result().composerTarget).toEqual(queued(5));
	});

	it("a failed begin returns the composer to the row it followed before Edit", async () => {
		const t = setup({ storeQueue: [mockRow5, mockRow6Marked] });
		t.beginEdit(5);
		expect(t.result().composerTarget).toEqual(queued(5));

		await waitFor(() => expect(t.patches).toHaveLength(1));
		await t.settle(t.patches[0], "reject");
		await waitFor(() => expect(t.result().composerTarget).toEqual(queued(6)));
		expect(t.result().composerMode).toBe("follow");
	});

	it("the failure of a begin that a later begin superseded does not affect the composer", async () => {
		const t = setup({ storeQueue: [mockRow5, mockRow6] });
		t.beginEdit(5);
		t.beginEdit(6);
		expect(t.result().composerTarget).toEqual(queued(6));

		// The begin for row 6 runs once the one for row 5 settles.
		await waitFor(() => expect(t.patches).toHaveLength(1));
		await t.settle(t.patches[0], "reject");
		await waitFor(() => expect(t.patches).toHaveLength(2));
		expect(t.result().composerTarget).toEqual(queued(6));
		expect(t.result().markedID).toBe(6);

		await t.settle(t.patches[1], "reject");
		await waitFor(() => expect(t.result().composerTarget).toBeNull());
	});
});
