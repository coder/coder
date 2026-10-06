import type { WorkingBlock } from "./workingBlockGrouping";

type LiveBlockIdentity = {
	itemKey: string;
	firstMemberId: number | undefined;
	streamStartedAt: string | undefined;
};

const continuesLiveBlock = (
	block: WorkingBlock,
	identity: LiveBlockIdentity,
	streamStartedAt: string | undefined,
): boolean => {
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

export const emptyLiveBlockKeys: LiveBlockKeys = {
	itemKeys: new Map(),
	identity: null,
};

/**
 * Scroller item keys of blocks that rendered live, kept once they complete or
 * paging re-keys them, so an open block does not remount.
 */
export const reconcileLiveBlockKeys = (
	blocks: readonly WorkingBlock[],
	streamStartedAt: string | undefined,
	state: LiveBlockKeys,
): LiveBlockKeys => {
	const { identity } = state;
	let { itemKeys: nextItemKeys, identity: nextIdentity } = state;

	// Head ordinals shift when an older page reveals an earlier block of the
	// turn, so a live key passes by name only once nothing continues the live block.
	const liveBlockContinues =
		identity !== null &&
		blocks.some((block) =>
			continuesLiveBlock(block, identity, streamStartedAt),
		);

	for (const block of blocks) {
		let itemKey = nextItemKeys.get(block.key);
		if (itemKey === undefined) {
			if (
				nextIdentity !== null &&
				continuesLiveBlock(block, nextIdentity, streamStartedAt)
			) {
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
