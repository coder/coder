import type { WorkingBlock } from "./workingBlockGrouping";

type LiveBlockIdentity = {
	itemKey: string;
	firstPieceKey: string | undefined;
	livePiece: number | undefined;
	streamStartedAt: string | undefined;
};

const continuesLiveBlock = (
	block: WorkingBlock,
	identity: LiveBlockIdentity,
	streamStartedAt: string | undefined,
): boolean => {
	// A block keeps its persisted pieces, so once it has one no other block
	// continues it, even a new live block of the same stream.
	if (identity.firstPieceKey !== undefined) {
		return block.pieceKeys.includes(identity.firstPieceKey);
	}

	if (identity.streamStartedAt === undefined) {
		return false;
	}

	// Output can close a live-only block mid-stream and the next work opens
	// another, so the stream's blocks tell apart by their piece of it.
	if (identity.streamStartedAt === streamStartedAt) {
		return (
			identity.livePiece !== undefined && block.livePiece === identity.livePiece
		);
	}

	// A live-only block's first persisted step keeps its streamed part
	// timestamps, even when the stream clears in the same render.
	return (
		block.isLive && block.startedAt === Date.parse(identity.streamStartedAt)
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
 * Keeps the scroller item keys of blocks that rendered live once they complete
 * or paging re-keys them, so an open block does not remount. Returns `state`
 * itself when nothing changed, so callers can set it during render.
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

		const firstPieceKey = block.pieceKeys[0];
		if (
			block.isLive &&
			(nextIdentity?.itemKey !== itemKey ||
				nextIdentity.firstPieceKey !== firstPieceKey ||
				nextIdentity.livePiece !== block.livePiece ||
				nextIdentity.streamStartedAt !== streamStartedAt)
		) {
			nextIdentity = {
				itemKey,
				firstPieceKey,
				livePiece: block.livePiece,
				streamStartedAt,
			};
		}
	}

	return nextItemKeys === state.itemKeys && nextIdentity === state.identity
		? state
		: { itemKeys: nextItemKeys, identity: nextIdentity };
};
