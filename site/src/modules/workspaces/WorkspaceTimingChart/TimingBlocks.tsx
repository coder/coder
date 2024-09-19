import type { Interpolation, Theme } from "@emotion/react";
import { MoreHorizOutlined } from "@mui/icons-material";
import type { FC } from "react";
import type { Timing } from "./timings";

const blocksPadding = 8;
const blocksSpacing = 4;
const moreIconSize = 18;

type TimingBlocksProps = {
	timings: Timing[];
	stageSize: number;
	blockSize: number;
};

export const TimingBlocks: FC<TimingBlocksProps> = ({
	timings,
	stageSize,
	blockSize,
}) => {
	const spacingBetweenBlocks = (timings.length - 1) * blocksSpacing;
	const freeSize = stageSize - blocksPadding * 2;
	const necessarySize = blockSize * timings.length + spacingBetweenBlocks;
	const hasSpacing = necessarySize <= freeSize;
	const nOfPossibleBlocks = Math.floor(
		(freeSize - moreIconSize - spacingBetweenBlocks) / blockSize,
	);
	const nOfBlocks = hasSpacing ? timings.length : nOfPossibleBlocks;

	return (
		<div css={styles.blocks}>
			{Array.from({ length: nOfBlocks }).map((_, i) => (
				// biome-ignore lint/suspicious/noArrayIndexKey: we are using the index as a key here because the blocks are not expected to be reordered
				<div key={i} css={styles.block} style={{ minWidth: blockSize }} />
			))}
			{!hasSpacing && (
				<div css={styles.extraBlock}>
					<MoreHorizOutlined />
				</div>
			)}
		</div>
	);
};

const styles = {
	blocks: {
		display: "flex",
		width: "100%",
		height: "100%",
		padding: blocksPadding,
		gap: blocksSpacing,
		alignItems: "center",
	},
	block: {
		borderRadius: 4,
		height: 16,
		backgroundColor: "#082F49",
		border: "1px solid #38BDF8",
		flexShrink: 0,
	},
	extraBlock: {
		color: "#38BDF8",
		lineHeight: 0,
		flexShrink: 0,

		"& svg": {
			fontSize: moreIconSize,
		},
	},
} satisfies Record<string, Interpolation<Theme>>;
