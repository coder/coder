/**
 * Copied from shadc/ui on 04/18/2025
 * @see {@link https://ui.shadcn.com/docs/components/textarea}
 */
import * as React from "react";

import { cn } from "utils/cn";

export const Textarea = React.forwardRef<
	HTMLTextAreaElement,
	React.ComponentProps<"textarea">
>(({ className, ...props }, ref) => {
	return (
		<textarea
			className={cn(
				`flex min-h-[60px] w-full px-3 py-2 text-sm shadow-sm text-content-primary
				rounded-md border border-border bg-transparent placeholder:text-content-secondary
				focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
				disabled:cursor-not-allowed disabled:opacity-50 disabled:text-content-disabled md:text-sm`,
				className,
			)}
			ref={ref}
			{...props}
		/>
	);
});
