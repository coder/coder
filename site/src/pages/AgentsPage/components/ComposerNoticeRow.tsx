import { cva, type VariantProps } from "class-variance-authority";
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

type ComposerNoticeRowProps = Required<
	VariantProps<typeof composerNoticeRowVariants>
> & {
	id: string;
	children: React.ReactNode;
};

/** A notice row with a warning icon, stacked at the top of the chat composer. */
export const ComposerNoticeRow: React.FC<ComposerNoticeRowProps> = ({
	id,
	tone,
	children,
}) => {
	return (
		<div id={id} className={composerNoticeRowVariants({ tone })}>
			<TriangleAlertIcon className="mt-px size-3.5 shrink-0 text-content-warning" />
			{children}
		</div>
	);
};
