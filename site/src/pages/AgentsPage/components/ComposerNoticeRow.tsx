import { cva } from "class-variance-authority";
import { cn } from "cn";
import { TriangleAlertIcon } from "lucide-react";

const composerNoticeRowVariants = cva(
	"flex items-start gap-1.5 border-b border-border/70 px-3 py-1.5 text-xs",
	{
		variants: {
			tone: {
				warning: "font-medium text-content-warning",
				secondary: "text-content-secondary",
			},
		},
	},
);

type ComposerNoticeRowProps = {
	id: string;
	tone: "warning" | "secondary";
	children: React.ReactNode;
};

/** A warning row stacked at the top of the chat composer. */
export const ComposerNoticeRow: React.FC<ComposerNoticeRowProps> = ({
	id,
	tone,
	children,
}) => {
	return (
		<div id={id} className={composerNoticeRowVariants({ tone })}>
			<TriangleAlertIcon
				className={cn(
					"mt-px size-3.5 shrink-0",
					tone === "secondary" && "text-content-warning",
				)}
			/>
			{children}
		</div>
	);
};
