import { MemoizedMarkdown } from "components/Markdown/Markdown";
import { forwardRef } from "react";
import { cn } from "utils/cn";

interface ResponseProps
	extends Omit<React.HTMLAttributes<HTMLDivElement>, "children"> {
	children: string;
}

export const Response = forwardRef<HTMLDivElement, ResponseProps>(
	({ className, children, ...props }, ref) => {
		return (
			<div
				ref={ref}
				className={cn("text-sm leading-relaxed text-content-primary", className)}
				{...props}
			>
				<MemoizedMarkdown>{children}</MemoizedMarkdown>
			</div>
		);
	},
);

Response.displayName = "Response";
