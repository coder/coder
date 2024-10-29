import type { Interpolation, Theme } from "@emotion/react";
import MoreHorizOutlined from "@mui/icons-material/MoreHorizOutlined";
import { type FC, useEffect, useRef, useState } from "react";

const spaceBetweenBlocks = 4;
const moreIconSize = 18;
const blockSize = 20;

type BlocksProps = {
	count: number;
};

export const Blocks: FC<BlocksProps> = ({ count }) => {
	const [availableWidth, setAvailableWidth] = useState<number>(0);
	const blocksRef = useRef<HTMLDivElement>(null);

	// Fix: When using useLayoutEffect, Chromatic fails to calculate the right width.
	useEffect(() => {
		if (availableWidth || !blocksRef.current) {
			return;
		}
		setAvailableWidth(blocksRef.current.clientWidth);
	}, [availableWidth]);

	const totalSpaceBetweenBlocks = (count - 1) * spaceBetweenBlocks;
	const necessarySize = blockSize * count + totalSpaceBetweenBlocks;
	const hasSpacing = necessarySize <= availableWidth;
	const nOfPossibleBlocks = Math.floor(
		(availableWidth - moreIconSize) / (blockSize + spaceBetweenBlocks),
	);
	const nOfBlocks = hasSpacing ? count : nOfPossibleBlocks;

	return (
		<div ref={blocksRef} css={styles.blocks}>
			{Array.from({ length: nOfBlocks }, (_, i) => i + 1).map((n) => (
				<div key={n} css={styles.block} style={{ minWidth: blockSize }} />
			))}
			{!hasSpacing && (
				<div css={styles.more}>
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
		gap: spaceBetweenBlocks,
		alignItems: "center",
	},
	block: (theme) => ({
		borderRadius: 4,
		height: 18,
		backgroundColor: theme.roles.active.background,
		border: `1px solid ${theme.roles.active.outline}`,
		flexShrink: 0,
		flex: 1,
	}),
	more: (theme) => ({
		color: theme.roles.active.outline,
		lineHeight: 0,
		flexShrink: 0,
		flex: 1,

		"& svg": {
			fontSize: moreIconSize,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
