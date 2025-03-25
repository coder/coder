import type { FC, HTMLProps } from "react";
import { cn } from "utils/cn";

type UnreadBadgeProps = {
	count: number;
} & HTMLProps<HTMLSpanElement>;

export const UnreadBadge: FC<UnreadBadgeProps> = ({
	count,
	className,
	...props
}) => {
	return (
		<span
			className={cn([
				"flex min-w-[18px] h-[18px] w-fit px-1 rounded text-2xs items-center justify-center",
				"bg-surface-sky text-highlight-sky",
				className,
			])}
			{...props}
		>
			{count > 99 ? "99+" : count}
		</span>
	);
};
