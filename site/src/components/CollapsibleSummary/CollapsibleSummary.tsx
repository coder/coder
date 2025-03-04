import { type VariantProps, cva } from "class-variance-authority";
import { ChevronRightIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { cn } from "utils/cn";

const collapsibleSummaryVariants = cva(
	`flex items-center gap-1 p-0 bg-transparent border-0 text-inherit cursor-pointer
	transition-colors text-content-secondary hover:text-content-primary font-medium
	whitespace-nowrap`,
	{
		variants: {
			size: {
				md: "text-sm",
				sm: "text-xs",
			},
		},
		defaultVariants: {
			size: "md",
		},
	},
);

export interface CollapsibleSummaryProps
	extends VariantProps<typeof collapsibleSummaryVariants> {
	/**
	 * The label to display for the collapsible section
	 */
	label: string;
	/**
	 * The content to show when expanded
	 */
	children: ReactNode;
	/**
	 * Whether the section is initially expanded
	 */
	defaultOpen?: boolean;
	/**
	 * Optional className for the button
	 */
	className?: string;
	/**
	 * The size of the component
	 */
	size?: "md" | "sm";
}

export const CollapsibleSummary: FC<CollapsibleSummaryProps> = ({
	label,
	children,
	defaultOpen = false,
	className,
	size,
}) => {
	const [isOpen, setIsOpen] = useState(defaultOpen);

	return (
		<div className="flex flex-col gap-4">
			<button
				className={cn(
					collapsibleSummaryVariants({ size }),
					isOpen && "text-content-primary",
					className,
				)}
				type="button"
				onClick={() => {
					setIsOpen((v) => !v);
				}}
			>
				<div
					className={cn(
						"flex items-center justify-center transition-transform duration-200",
						isOpen ? "rotate-90" : "rotate-0",
					)}
				>
					<ChevronRightIcon
						className={cn(
							"p-0.5",
							size === "sm" ? "size-icon-xs" : "size-icon-sm",
						)}
					/>
				</div>
				<span className="sr-only">
					({isOpen ? "Hide" : "Show"}) {label}
				</span>
				<span className="[&:first-letter]:uppercase">{label}</span>
			</button>

			{isOpen && <div className="flex flex-col gap-4">{children}</div>}
		</div>
	);
};
