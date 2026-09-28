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
	deriveQueuedMessageUnderEditID,
	type MarkerRequest,
	useQueuedMessageEdit,
} from "./useQueuedMessageEdit";

const row5 = { ...MockChatQueuedMessage, id: 5 };
const row5Marked = { ...MockChatQueuedMessageUnderEdit, id: 5 };
const row6 = { ...MockChatQueuedMessage, id: 6 };
const row6Marked = { ...MockChatQueuedMessageUnderEdit, id: 6 };

const idle: MarkerRequest = { isPending: false, variables: undefined };
const pendingBegin = (id: number): MarkerRequest => ({
	isPending: true,
	variables: { queuedMessageId: id, req: { editing: true } },
});
const pendingEnd = (id: number): MarkerRequest => ({
	isPending: true,
	variables: { queuedMessageId: id, req: { editing: false } },
});
const settledBegin = (id: number): MarkerRequest => ({
	isPending: false,
	variables: { queuedMessageId: id, req: { editing: true } },
});

const queued = (id: number) => ({ kind: "queued", id }) as const;
const history = (id: number) => ({ kind: "history", id }) as const;

describe("the row shown as under edit", () => {
	it.each([
		["the marked row shows as under edit", 5, idle, 5],
		[
			"a pending begin shows its row, even while another row is marked",
			5,
			pendingBegin(6),
			6,
		],
		["a pending end on the marked row hides it", 5, pendingEnd(5), null],
		["a pending end on another row leaves the marked row", 5, pendingEnd(6), 5],
		["a settled begin leaves it to the marked row", 6, settledBegin(5), 6],
	])("%s", (_name, serverMarkedID, marker, expected) => {
		expect(deriveQueuedMessageUnderEditID(serverMarkedID, marker)).toBe(
			expected,
		);
	});
});

describe("the row the composer edits", () => {
	const owner = true;
	const viewer = false;
	const cases: Array<
		[string, ComposerMode, number | null, MarkerRequest, boolean, unknown]
	> = [
		[
			"an untouched composer follows the marked row",
			"follow",
			5,
			idle,
			owner,
			queued(5),
		],
		[
			"an untouched composer with no marked row edits nothing",
			"follow",
			null,
			idle,
			owner,
			null,
		],
		[
			"a viewer's untouched composer never follows the marked row",
			"follow",
			5,
			idle,
			viewer,
			null,
		],
		["a draft ignores the marked row", "draft", 5, idle, owner, null],
		[
			"a history choice stays in effect",
			history(3),
			5,
			idle,
			owner,
			history(3),
		],
		[
			"a queued choice stays in effect while the server marks that row",
			queued(5),
			5,
			idle,
			owner,
			queued(5),
		],
		[
			"a queued choice stays in effect while its begin is pending, even if another row is marked",
			queued(5),
			6,
			pendingBegin(5),
			owner,
			queued(5),
		],
		[
			"a queued choice closes once the server no longer marks its row",
			queued(5),
			6,
			settledBegin(5),
			owner,
			null,
		],
	];
	it.each(cases)(
		"%s",
		(_name, composerMode, serverMarkedID, marker, isOwner, expected) => {
			expect(
				deriveComposerTarget(composerMode, serverMarkedID, marker, isOwner),
			).toEqual(expected);
		},
	);
});

type Patch = {
	id: number;
	req: TypesGen.EditChatQueuedMessageRequest;
	resolve: () => void;
	reject: (error: Error) => void;
};

// The marker mutation is real so the hook sees the same pending, error and
// variables transitions the page does, including a superseded call.
const setup = (options?: {
	storeQueue?: readonly TypesGen.ChatQueuedMessage[];
	composerMode?: ComposerMode;
}) => {
	const store = createChatStore();
	store.setQueuedMessages(options?.storeQueue ?? []);
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
				marker: {
					isPending: marker.isPending,
					variables: marker.variables,
				},
				setQueuedMessageEditing: (id, editing) =>
					marker.mutateAsync({ queuedMessageId: id, req: { editing } }),
			});
			return { ...edit, composerMode, markerStatus: marker.status };
		},
		{
			wrapper: ({ children }: { children: React.ReactNode }) =>
				createElement(QueryClientProvider, { client: queryClient }, children),
		},
	);
	const result = () => hook.result.current;
	// react-query starts the mutationFn and notifies observers on later
	// ticks, so each step waits for what it changes.
	const beginEdit = async (id: number) => {
		act(() => result().handleEditQueuedMessage(id));
		await waitFor(() => expect(result().queuedMessageUnderEditID).toBe(id));
	};
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

		act(() => t.store.setQueuedMessages([row5Marked]));
		expect(t.result().composerTarget).toEqual(queued(5));
		expect(t.result().composerMode).toBe("follow");
	});

	it("Edit opens the composer and shows the row as under edit before the begin request settles", async () => {
		const t = setup({ storeQueue: [row5] });

		await t.beginEdit(5);
		await waitFor(() => expect(t.patches).toHaveLength(1));
		expect(t.patches.map(({ id, req }) => ({ id, req }))).toEqual([
			{ id: 5, req: { editing: true } },
		]);
		expect(t.result().composerTarget).toEqual(queued(5));

		act(() => t.store.setQueuedMessages([row5Marked]));
		await t.settle(t.patches[0], "resolve");
		await waitFor(() => expect(t.result().markerStatus).toBe("success"));
		expect(t.result().composerTarget).toEqual(queued(5));
	});

	it("a failed begin closes the edit", async () => {
		const t = setup({ storeQueue: [row5, row6Marked] });
		await t.beginEdit(5);
		expect(t.result().composerTarget).toEqual(queued(5));

		await waitFor(() => expect(t.patches).toHaveLength(1));
		await t.settle(t.patches[0], "reject");
		await waitFor(() => expect(t.result().composerTarget).toBeNull());
	});

	it("the failure of a begin that a later begin superseded does not affect the composer", async () => {
		const t = setup({ storeQueue: [row5, row6] });
		await t.beginEdit(5);
		await t.beginEdit(6);
		expect(t.result().composerTarget).toEqual(queued(6));

		// The begin for row 6 runs once the one for row 5 settles.
		await waitFor(() => expect(t.patches).toHaveLength(1));
		await t.settle(t.patches[0], "reject");
		await waitFor(() => expect(t.patches).toHaveLength(2));
		expect(t.result().composerTarget).toEqual(queued(6));
		expect(t.result().queuedMessageUnderEditID).toBe(6);

		await t.settle(t.patches[1], "reject");
		await waitFor(() => expect(t.result().composerTarget).toBeNull());
	});
});
