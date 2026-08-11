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
				"pointer-events-none bg-[image:var(--supergraphic-url)] bg-[length:100%_100%] bg-no-repeat",
				className,
			)}
			{...props}
		/>
	);
};
