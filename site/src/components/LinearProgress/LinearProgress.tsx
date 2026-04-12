import type React from "react";
import type { FC } from "react";
import { cn } from "#/utils/cn";

type LinearProgressProps = React.ComponentProps<"div"> & {
	value: number;
	variant: "determinate" | "indeterminate";
};

const LinearProgress: FC<LinearProgressProps> = ({
	value,
	className,
	variant,
	...props
}) => {
	return (
		<div
			className={cn(
				"w-full h-1 bg-surface-sky rounded-full relative",
				"overflow-hidden block",
				className,
			)}
			{...props}
		>
			{variant === "indeterminate" ? (
				<>
					<div
						className={cn(
							"absolute inset-y-0 w-auto origin-left rounded-full bg-highlight-sky",
							"animate-bar-indeterminate",
						)}
					/>
					<div
						className={cn(
							"absolute inset-y-0 w-auto origin-left rounded-full bg-highlight-sky",
							"animate-bar-indeterminate-2",
						)}
					/>
				</>
			) : (
				<div
					className="h-full rounded-full bg-highlight-sky transition-[width] duration-200 ease-linear"
					style={{ width: `${value}%` }}
				/>
			)}
		</div>
	);
};

export default LinearProgress;
