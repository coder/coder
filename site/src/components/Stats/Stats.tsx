import type { FC, HTMLAttributes, ReactNode } from "react";
import { cn } from "utils/cn";

export const Stats: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<div
			className={cn(
				"p-4 rounded-[8px] block flex-wrap items-center m-0 text-content-secondary border border-solid border-border text-sm leading-relaxed font-normal md:py-0 md:flex",
				className,
			)}
			{...attrs}
		>
			{children}
		</div>
	);
};

interface StatsItemProps extends HTMLAttributes<HTMLDivElement> {
	label: string;
	value: ReactNode;
}

export const StatsItem: FC<StatsItemProps> = ({
	label,
	value,
	className,
	...attrs
}) => {
	return (
		<div
			className={cn(
				"text-sm p-2 flex items-baseline gap-2 md:py-3.5 md:px-4",
				className,
			)}
			{...attrs}
		>
			<span className="block break-words">{label}:</span>
			<span className="flex items-center break-words text-content-primary [&_a]:text-content-primary [&_a]:no-underline [&_a]:font-semibold [&_a:hover]:no-underline">
				{value}
			</span>
		</div>
	);
};
