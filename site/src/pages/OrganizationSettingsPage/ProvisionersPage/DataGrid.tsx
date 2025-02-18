import type { FC, HTMLProps } from "react";
import { cn } from "utils/cn";

export const DataGrid: FC<HTMLProps<HTMLDListElement>> = ({
	className,
	...props
}) => {
	return (
		<dl
			{...props}
			className={cn([
				"m-0 grid grid-cols-[auto_1fr] gap-x-4 items-center",
				"[&_dd]:text-content-primary [&_dd]:font-mono [&_dd]:leading-[22px]",
				className,
			])}
		/>
	);
};

export const DataGridSpace: FC<HTMLProps<HTMLDivElement>> = ({
	className,
	...props
}) => {
	return <div {...props} className={cn(["h-6 col-span-2", className])} />;
};
