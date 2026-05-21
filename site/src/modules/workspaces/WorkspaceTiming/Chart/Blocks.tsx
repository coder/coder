import { EllipsisIcon } from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { cn } from "#/utils/cn";

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
	const nOfPossibleBlocks = Math.max(
		0,
		Math.floor(
			(availableWidth - moreIconSize) / (blockSize + spaceBetweenBlocks),
		),
	);
	const nOfBlocks = hasSpacing ? count : nOfPossibleBlocks;

	return (
		<div ref={blocksRef} className="flex h-full w-full items-center gap-1">
			{Array.from({ length: nOfBlocks }, (_, i) => i + 1).map((n) => (
				<div
					key={n}
					className={cn(
						"h-[18px] min-w-5 shrink-0 flex-1 rounded",
						"border border-solid border-content-link bg-content-link/15",
					)}
				/>
			))}
			{!hasSpacing && (
				<div className="shrink-0 flex-1 leading-none text-content-link">
					<EllipsisIcon className="size-[18px]" />
				</div>
			)}
		</div>
	);
};
