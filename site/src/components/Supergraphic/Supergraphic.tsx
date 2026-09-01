import type { FC } from "react";
import { cn } from "#/utils/cn";

/**
 * A decorative layer that paints the Coder brand supergraphic.
 */
export const Supergraphic: FC<React.ComponentProps<"div">> = ({
	className,
	...props
}) => {
	return (
		<div
			aria-hidden="true"
			className={cn(
				"absolute inset-0 -z-10 pointer-events-none bg-(image:--supergraphic-url) bg-size-[100%_100%] bg-no-repeat",
				className,
			)}
			{...props}
		/>
	);
};
