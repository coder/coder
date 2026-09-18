import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatQueuedMessage,
	MockChatQueuedMessageUnderEdit,
} from "#/testHelpers/chatEntities";
import { createChatStore } from "../components/ChatConversation/chatStore";
import type { EditingTarget } from "../components/ChatConversation/types";
import { useQueuedMessageEdit } from "./useQueuedMessageEdit";

const row5 = { ...MockChatQueuedMessage, id: 5 };
const row5Marked = { ...MockChatQueuedMessageUnderEdit, id: 5 };
const row6 = { ...MockChatQueuedMessage, id: 6 };
const row6Marked = { ...MockChatQueuedMessageUnderEdit, id: 6 };

type Patch = {
	id: number;
	req: TypesGen.EditChatQueuedMessageRequest;
	resolve: () => void;
	reject: (error: Error) => void;
};

const setup = (options?: {
	storeQueue?: readonly TypesGen.ChatQueuedMessage[];
	restoreQueue?: readonly TypesGen.ChatQueuedMessage[];
	ready?: boolean;
	allowed?: boolean;
	editingTarget?: EditingTarget | null;
}) => {
	const store = createChatStore();
	store.setQueuedMessages(options?.storeQueue ?? []);
	const composer = {
		editingTarget: options?.editingTarget ?? null,
		handleBeginEdit: vi.fn(),
		handleCancelEdit: vi.fn(() => {
			composer.editingTarget = null;
		}),
		leaveEditKeepingText: vi.fn(() => {
			composer.editingTarget = null;
		}),
	};
	const beginEditFromRow = vi.fn((row: TypesGen.ChatQueuedMessage) => {
		composer.editingTarget = { kind: "queued", id: row.id };
	});
	const patches: Patch[] = [];
	const patchQueuedMessage = vi.fn(
		(id: number, req: TypesGen.EditChatQueuedMessageRequest) =>
			new Promise<void>((resolve, reject) => {
				patches.push({ id, req, resolve, reject });
			}),
	);
	const restore = {
		queuedMessages: options?.restoreQueue,
		ready: options?.ready ?? true,
		allowed: options?.allowed ?? true,
	};
	const hook = renderHook(() =>
		useQueuedMessageEdit({
			store,
			composer: { ...composer },
			beginEditFromRow,
			patchQueuedMessage,
			restore: { ...restore },
		}),
	);
	// Composer state lives outside the hook, so a change becomes visible on
	// the next render, like the page's own state.
	const sync = () => act(() => hook.rerender());
	const beginEdit = (id: number) => {
		act(() => result().handleEditQueuedMessage(id));
		sync();
	};
	const settle = async (patch: Patch, outcome: "resolve" | "reject") => {
		await act(async () => {
			if (outcome === "resolve") {
				patch.resolve();
			} else {
				patch.reject(new Error("request failed"));
			}
		});
		sync();
	};
	const result = () => hook.result.current;
	return {
		store,
		composer,
		beginEditFromRow,
		patchQueuedMessage,
		patches,
		restore,
		result,
		sync,
		beginEdit,
		settle,
	};
};

describe("useQueuedMessageEdit", () => {
	it("opens the composer before the begin request settles and keeps it once the marker is seen", async () => {
		const t = setup({ storeQueue: [row5], restoreQueue: [row5] });

		t.beginEdit(5);
		expect(t.beginEditFromRow).toHaveBeenCalledWith(row5);
		expect(t.patchQueuedMessage).toHaveBeenCalledWith(
			5,
			{ editing: true },
			"Failed to start editing the queued message.",
		);
		expect(t.result().localQueuedEditMarker).toEqual({ id: 5, editing: true });

		act(() => t.store.setQueuedMessages([row5Marked]));
		await t.settle(t.patches[0], "resolve");
		expect(t.result().queuedEditTargetID).toBe(5);
		expect(t.composer.handleCancelEdit).not.toHaveBeenCalled();
		expect(t.composer.leaveEditKeepingText).not.toHaveBeenCalled();
	});

	it("keeps the typed text when the server clears a marker it had shown", () => {
		const t = setup({ storeQueue: [row5], restoreQueue: [row5] });
		t.beginEdit(5);
		act(() => t.store.setQueuedMessages([row5Marked]));

		act(() => t.store.setQueuedMessages([row5]));
		t.sync();
		expect(t.composer.leaveEditKeepingText).toHaveBeenCalledTimes(1);
		expect(t.composer.handleCancelEdit).not.toHaveBeenCalled();
		expect(t.result().queuedEditTargetID).toBeNull();
	});

	it("restores the draft when the row leaves the queue before its marker was seen", () => {
		const t = setup({ storeQueue: [row5], restoreQueue: [row5] });
		t.beginEdit(5);

		act(() => t.store.setQueuedMessages([]));
		t.sync();
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(1);
		expect(t.composer.leaveEditKeepingText).not.toHaveBeenCalled();
	});

	it("restores the draft when the begin fails and no row is marked on the server", async () => {
		const t = setup({ storeQueue: [row5], restoreQueue: [row5] });
		t.beginEdit(5);

		await t.settle(t.patches[0], "reject");
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(1);
		expect(t.result().queuedEditTargetID).toBeNull();
	});

	it("follows the row the server marks when the begin fails", async () => {
		const t = setup({
			storeQueue: [row5, row6Marked],
			restoreQueue: [row5, row6],
		});
		t.beginEdit(5);
		expect(t.beginEditFromRow).toHaveBeenLastCalledWith(row5);

		await t.settle(t.patches[0], "reject");
		expect(t.beginEditFromRow).toHaveBeenLastCalledWith(row6Marked);
		expect(t.composer.handleCancelEdit).not.toHaveBeenCalled();
		expect(t.result().queuedEditTargetID).toBe(6);
	});

	it("ignores the failure of a begin that a later begin superseded", async () => {
		const t = setup({ storeQueue: [row5], restoreQueue: [row5] });
		t.beginEdit(5);
		act(() => {
			void t
				.result()
				.handleEndQueuedMessageEdit(5)
				.catch(() => undefined);
		});
		t.sync();
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(1);
		t.beginEdit(5);
		expect(t.patches.map((patch) => patch.req)).toEqual([
			{ editing: true },
			{ editing: false },
			{ editing: true },
		]);

		await t.settle(t.patches[0], "reject");
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(1);
		expect(t.result().queuedEditTargetID).toBe(5);

		await t.settle(t.patches[2], "reject");
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(2);
	});

	it("reports the local marker as editing for the target, then as ended until the snapshot unmarks the row", async () => {
		const t = setup({ storeQueue: [row5], restoreQueue: [row5] });
		t.beginEdit(5);
		act(() => t.store.setQueuedMessages([row5Marked]));
		await t.settle(t.patches[0], "resolve");
		expect(t.result().localQueuedEditMarker).toEqual({ id: 5, editing: true });

		let ended: Promise<void> | undefined;
		act(() => {
			ended = t.result().handleEndQueuedMessageEdit(5);
		});
		t.sync();
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(1);
		expect(t.patchQueuedMessage).toHaveBeenLastCalledWith(
			5,
			{ editing: false },
			"Failed to cancel the edit.",
		);
		expect(t.result().localQueuedEditMarker).toEqual({
			id: 5,
			editing: false,
		});

		act(() => t.store.setQueuedMessages([row5]));
		await t.settle(t.patches[1], "resolve");
		await ended;
		expect(t.result().localQueuedEditMarker).toBeUndefined();
	});

	it("drops the ended marker when the end request fails", async () => {
		const t = setup({ storeQueue: [row5Marked], restoreQueue: [row5Marked] });
		t.sync();
		expect(t.result().queuedEditTargetID).toBe(5);

		let endedWith: Promise<unknown> | undefined;
		act(() => {
			endedWith = t
				.result()
				.handleEndQueuedMessageEdit(5)
				.catch((error: unknown) => error);
		});
		t.sync();
		expect(t.result().localQueuedEditMarker).toEqual({
			id: 5,
			editing: false,
		});

		await t.settle(t.patches[0], "reject");
		await expect(endedWith).resolves.toEqual(new Error("request failed"));
		expect(t.result().localQueuedEditMarker).toBeUndefined();
	});

	it("restores a marked row once when the first fetched queue is ready", () => {
		const t = setup({
			storeQueue: [row5Marked],
			restoreQueue: [row5Marked],
			ready: false,
		});
		expect(t.beginEditFromRow).not.toHaveBeenCalled();

		t.restore.ready = true;
		t.sync();
		expect(t.beginEditFromRow).toHaveBeenCalledTimes(1);
		expect(t.beginEditFromRow).toHaveBeenCalledWith(row5Marked);

		t.sync();
		t.sync();
		expect(t.beginEditFromRow).toHaveBeenCalledTimes(1);
	});

	it("does not restore a marked row for a viewer", () => {
		const t = setup({
			storeQueue: [row5Marked],
			restoreQueue: [row5Marked],
			allowed: false,
		});
		t.sync();
		expect(t.beginEditFromRow).not.toHaveBeenCalled();
		expect(t.result().queuedEditTargetID).toBeNull();
	});

	it("returns to the server-marked row when a history edit is cancelled", () => {
		const t = setup({
			storeQueue: [row5Marked],
			restoreQueue: [row5Marked],
			editingTarget: { kind: "history", id: 3 },
		});
		expect(t.beginEditFromRow).not.toHaveBeenCalled();

		act(() => t.result().handleCancelEdit());
		expect(t.beginEditFromRow).toHaveBeenCalledWith(row5Marked);
		expect(t.composer.handleCancelEdit).not.toHaveBeenCalled();
	});

	it("restores the draft when a history edit is cancelled and no row is marked", () => {
		const t = setup({
			storeQueue: [row5],
			restoreQueue: [row5],
			editingTarget: { kind: "history", id: 3 },
		});

		act(() => t.result().handleCancelEdit());
		expect(t.beginEditFromRow).not.toHaveBeenCalled();
		expect(t.composer.handleCancelEdit).toHaveBeenCalledTimes(1);
	});
});
