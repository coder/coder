import type { Interpolation, Theme } from "@emotion/react";
import { MoreHorizOutlined } from "@mui/icons-material";
import type { FC } from "react";

const sidePadding = 8;
const spaceBetweenBlocks = 4;
const moreIconSize = 18;
const blockSize = 20;

type TimingBlocksProps = {
	count: number;
	size: number;
};

export const TimingBlocks: FC<TimingBlocksProps> = ({ count, size }) => {
	const totalSpaceBetweenBlocks = (count - 1) * spaceBetweenBlocks;
	const freeSize = size - sidePadding * 2;
	const necessarySize = blockSize * count + totalSpaceBetweenBlocks;
	const hasSpacing = necessarySize <= freeSize;
	const nOfPossibleBlocks = Math.floor(
		(freeSize - moreIconSize - totalSpaceBetweenBlocks) / blockSize,
	);
	const nOfBlocks = hasSpacing ? count : nOfPossibleBlocks;

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
		padding: sidePadding,
		gap: spaceBetweenBlocks,
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
