/**
 * @file Provides reusable vertical overflow behavior.
 */
import type { FC, ReactNode } from "react";

type OverflowYProps = {
	children?: ReactNode;
	className?: string;
	height?: number;
	maxHeight?: number;
};

export const OverflowY: FC<OverflowYProps> = ({
	children,
	height,
	maxHeight,
	...attrs
}) => {
	const computedHeight = height === undefined ? "100%" : `${height}px`;

	// Doing Math.max check to catch cases where height is accidentally larger
	// than maxHeight
	const computedMaxHeight =
		maxHeight === undefined
			? computedHeight
			: `${Math.max(height ?? 0, maxHeight)}px`;

	return (
		<div
			css={{
				height: computedHeight,
				maxHeight: computedMaxHeight,
			}}
			className="w-full overflow-y-auto flex-shrink-1"
			{...attrs}
		>
			{children}
		</div>
	);
};
