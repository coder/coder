import type { Interpolation, Theme } from "@emotion/react";
import MoreHorizOutlined from "@mui/icons-material/MoreHorizOutlined";
import { type FC, useLayoutEffect, useRef, useState } from "react";

const sidePadding = 8;
const spaceBetweenBlocks = 4;
const moreIconSize = 18;
const blockSize = 20;

type BarBlocksProps = {
	count: number;
};

export const BarBlocks: FC<BarBlocksProps> = ({ count }) => {
	const [parentWidth, setParentWidth] = useState<number>();
	const blocksRef = useRef<HTMLDivElement>(null);

	useLayoutEffect(() => {
		if (!blocksRef.current || parentWidth) {
			return;
		}
		const parentEl = blocksRef.current.parentElement;
		if (!parentEl) {
			throw new Error("BarBlocks must be a child of a Bar");
		}
		setParentWidth(parentEl.clientWidth);
	}, [parentWidth]);

	const totalSpaceBetweenBlocks = (count - 1) * spaceBetweenBlocks;
	const freeSize = parentWidth ? parentWidth - sidePadding * 2 : 0;
	const necessarySize = blockSize * count + totalSpaceBetweenBlocks;
	const hasSpacing = necessarySize <= freeSize;
	const nOfPossibleBlocks = Math.floor(
		(freeSize - moreIconSize) / (blockSize + spaceBetweenBlocks),
	);
	const nOfBlocks = hasSpacing ? count : nOfPossibleBlocks;
	console.log("->", nOfBlocks, parentWidth, freeSize);

	return (
		<div ref={blocksRef} css={styles.blocks}>
			{Array.from({ length: nOfBlocks }, (_, i) => i + 1).map((n) => (
				<div key={n} css={styles.block} style={{ minWidth: blockSize }} />
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
