import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { ChevronRightIcon } from "lucide-react";
import { useRef, useState } from "react";
import { flushSync } from "react-dom";

const collapsibleSummaryVariants = cva(
	cn(
		"flex items-center gap-1 p-0 bg-transparent cursor-pointer",
		"transition-colors text-content-secondary hover:text-content-primary font-medium",
		"whitespace-nowrap",
	),
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

type CollapsibleSummaryProps = VariantProps<
	typeof collapsibleSummaryVariants
> & {
	/**
	 * The label to display for the collapsible section
	 */
	label: string;
	/**
	 * The content to show when expanded
	 */
	children: React.ReactNode;
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
	/**
	 * Will scroll the children into view whenever the component is opened
	 */
	scrollIntoViewOnOpen?: boolean;
};

export const CollapsibleSummary: React.FC<CollapsibleSummaryProps> = ({
	label,
	children,
	defaultOpen = false,
	className,
	size,
	scrollIntoViewOnOpen,
}) => {
	const [isOpen, setIsOpen] = useState(defaultOpen);

	const ref = useRef<HTMLDivElement>(null);

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
					// The children only exist once the open state renders.
					flushSync(() => {
						setIsOpen((v) => !v);
					});

					if (scrollIntoViewOnOpen) {
						ref.current?.scrollIntoView({ behavior: "smooth" });
					}
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

			{isOpen && (
				<div className="flex flex-col gap-4" ref={ref}>
					{children}
				</div>
			)}
		</div>
	);
};
