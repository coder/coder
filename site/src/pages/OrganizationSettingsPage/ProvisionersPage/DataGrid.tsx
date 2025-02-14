import type { FC, HTMLProps } from "react";
import { cn } from "utils/cn";

export const DataGrid: FC<HTMLProps<HTMLDivElement>> = ({
	className,
	...props
}) => {
	return (
		<div
			{...props}
			className={cn([
				"grid grid-cols-[auto_1fr] gap-x-4 items-center",
				"[&_span:nth-of-type(even)]:text-content-primary [&_span:nth-of-type(even)]:font-mono",
				"[&_span:nth-of-type(even)]:leading-[22px]",
				className,
			])}
		/>
	);
};

export const DataGridSpace: FC<HTMLProps<HTMLDivElement>> = ({
	className,
	...props
}) => {
	return (
		<div aria-hidden {...props} className={cn(["h-6 col-span-2", className])} />
	);
};
