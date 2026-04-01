import * as SeparatorPrimitive from "@radix-ui/react-separator";
/**
 * Copied from shadc/ui on 06/20/2025
 * @see {@link https://ui.shadcn.com/docs/components/separator}
 */
import type * as React from "react";
import { cn } from "utils/cn";

function Separator({
	className,
	orientation = "horizontal",
	decorative = true,
	...props
}: React.ComponentProps<typeof SeparatorPrimitive.Root>) {
	return (
		<SeparatorPrimitive.Root
			data-slot="separator"
			decorative={decorative}
			orientation={orientation}
			className={cn(
				"bg-border shrink-0 data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-px",
				className,
			)}
			{...props}
		/>
	);
}

export { Separator };
