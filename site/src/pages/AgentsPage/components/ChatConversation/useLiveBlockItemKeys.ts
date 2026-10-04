import { useState } from "react";
import type { WorkingBlock } from "./workingBlockGrouping";

type LiveBlockIdentity = {
	itemKey: string;
	firstMemberId: number | undefined;
	streamStartedAt: string | undefined;
};

const continuesLiveBlock = (
	block: WorkingBlock,
	identity: LiveBlockIdentity | null,
	streamStartedAt: string | undefined,
): identity is LiveBlockIdentity => {
	if (identity === null) {
		return false;
	}
	if (
		identity.firstMemberId !== undefined &&
		block.memberIds.includes(identity.firstMemberId)
	) {
		return true;
	}
	if (!block.isLive || identity.streamStartedAt === undefined) {
		return false;
	}
	if (identity.streamStartedAt === streamStartedAt) {
		return true;
	}
	// A live-only block's first persisted step keeps its streamed part
	// timestamps, even when the stream clears in the same render.
	return (
		identity.firstMemberId === undefined &&
		block.startedAt === Date.parse(identity.streamStartedAt)
	);
};

type LiveBlockKeys = {
	itemKeys: ReadonlyMap<string, string>;
	identity: LiveBlockIdentity | null;
};

const reconcile = (
	blocks: readonly WorkingBlock[],
	streamStartedAt: string | undefined,
	state: LiveBlockKeys,
): LiveBlockKeys => {
	let { itemKeys: nextItemKeys, identity: nextIdentity } = state;
	// Head ordinals shift when an older page reveals an earlier block of the
	// turn, so a live key passes by name only once nothing continues the live block.
	const liveBlockContinues = blocks.some((block) =>
		continuesLiveBlock(block, state.identity, streamStartedAt),
	);
	for (const block of blocks) {
		let itemKey = nextItemKeys.get(block.key);
		if (itemKey === undefined) {
			if (continuesLiveBlock(block, nextIdentity, streamStartedAt)) {
				itemKey = nextIdentity.itemKey;
			} else if (block.isLive) {
				itemKey = block.liveKey;
			} else if (!liveBlockContinues) {
				itemKey = nextItemKeys.get(block.liveKey);
			}
			if (itemKey === undefined) {
				continue;
			}
			nextItemKeys = new Map(nextItemKeys).set(block.key, itemKey);
		}
		const firstMemberId = block.memberIds[0];
		if (
			block.isLive &&
			(nextIdentity?.itemKey !== itemKey ||
				nextIdentity.firstMemberId !== firstMemberId ||
				nextIdentity.streamStartedAt !== streamStartedAt)
		) {
			nextIdentity = { itemKey, firstMemberId, streamStartedAt };
		}
	}
	return nextItemKeys === state.itemKeys && nextIdentity === state.identity
		? state
		: { itemKeys: nextItemKeys, identity: nextIdentity };
};

/**
 * Scroller item keys of blocks that rendered live, kept once they complete or
 * paging re-keys them, so an open block does not remount.
 */
export const useLiveBlockItemKeys = (
	workingBlocks: readonly WorkingBlock[],
	streamStartedAt: string | undefined,
): ReadonlyMap<string, string> => {
	const [state, setState] = useState<LiveBlockKeys>({
		itemKeys: new Map(),
		identity: null,
	});
	const next = reconcile(workingBlocks, streamStartedAt, state);
	if (next !== state) {
		setState(next);
	}
	return next.itemKeys;
};
