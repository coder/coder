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
): identity is LiveBlockIdentity =>
	identity !== null &&
	((identity.firstMemberId !== undefined &&
		block.memberIds.includes(identity.firstMemberId)) ||
		(block.isLive &&
			identity.streamStartedAt !== undefined &&
			identity.streamStartedAt === streamStartedAt));

type LiveBlockKeys = {
	itemKeys: ReadonlyMap<string, string>;
	identity: LiveBlockIdentity | null;
};

// Returns the input state when nothing changed so the caller can skip setState.
const reconcile = (
	blocks: readonly WorkingBlock[],
	streamStartedAt: string | undefined,
	state: LiveBlockKeys,
): LiveBlockKeys => {
	let { itemKeys: nextItemKeys, identity: nextIdentity } = state;
	// A completed block may take over a live key by name only while nothing
	// continues the live block itself. Head ordinals shift when an older page
	// reveals an earlier block of the same turn, so the revealed block would
	// otherwise alias the still-live one.
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
 * Scroller item keys of blocks that rendered live, kept once they complete so
 * the handoff does not remount an open block. Paging re-keys a live block
 * anchored on the head once its turn's prompt loads; its oldest member and
 * its stream both outlive the re-key, so either one identifies it.
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
