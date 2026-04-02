import { ChevronDownIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { cn } from "#/utils/cn";

interface CollapsibleSectionProps {
	title: string;
	description?: string;
	badge?: ReactNode;
	action?: ReactNode;
	defaultOpen?: boolean;
	/** "card" (default) renders a bordered card. "inline" renders a
	 *  lightweight divider-based section for use inside forms. */
	variant?: "card" | "inline";
	children: ReactNode;
}

export const CollapsibleSection: FC<CollapsibleSectionProps> = ({
	title,
	description,
	badge,
	action,
	defaultOpen,
	variant = "card",
	children,
}) => {
	const [open, setOpen] = useState(defaultOpen ?? true);

	if (variant === "inline") {
		return (
			<Collapsible open={open} onOpenChange={setOpen}>
				<div className="border-0 border-t border-solid border-border pt-4">
					<CollapsibleTrigger
						className={cn(
							"flex w-full cursor-pointer justify-between gap-4 rounded-md border-0 bg-transparent p-0 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
							description ? "items-start" : "items-center",
						)}
					>
						<div className="min-w-0 flex-1">
							<h3 className="m-0 text-sm font-medium text-content-primary">
								{title}
							</h3>
							{description && (
								<p className="m-0 text-xs text-content-secondary">
									{description}
								</p>
							)}
						</div>
						<div className="flex shrink-0 items-center gap-2">
							{action && (
								<div
									className="flex items-center"
									onClick={(e) => e.stopPropagation()}
									onKeyDown={(e) => e.stopPropagation()}
								>
									{action}
								</div>
							)}
							<ChevronDownIcon
								className={cn(
									"mt-0.5 h-4 w-4 shrink-0 text-content-secondary transition-transform duration-200",
									!open && "-rotate-90",
								)}
							/>
						</div>
					</CollapsibleTrigger>
					<CollapsibleContent>
						<div className="pt-3">{children}</div>
					</CollapsibleContent>
				</div>
			</Collapsible>
		);
	}

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<div className="rounded-lg border border-solid border-border-default">
				<CollapsibleTrigger
					className={cn(
						"flex w-full cursor-pointer justify-between gap-4 rounded-lg border-0 bg-transparent px-6 py-5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
						description ? "items-start" : "items-center",
					)}
				>
					<div className="min-w-0 flex-1">
						<div className="flex items-center gap-2">
							<h2 className="m-0 text-lg font-semibold text-content-primary">
								{title}
							</h2>
							{badge && <span className="[&>*]:ml-0">{badge}</span>}
						</div>
						{description && (
							<p className="m-0 mt-1 text-sm text-content-secondary">
								{description}
							</p>
						)}
					</div>
					<div className="flex shrink-0 items-center gap-2">
						{action && (
							<div
								className="flex items-center"
								onClick={(e) => e.stopPropagation()}
								onKeyDown={(e) => e.stopPropagation()}
							>
								{action}
							</div>
						)}
						<div className="flex h-5 items-center">
							<ChevronDownIcon
								className={cn(
									"h-4 w-4 text-content-secondary transition-transform duration-200",
									open && "rotate-180",
								)}
							/>
						</div>
					</div>
				</CollapsibleTrigger>
				<CollapsibleContent>
					<hr className="m-0 border-0 border-t border-solid border-border-default" />
					<div className="px-6 pb-5 pt-4">{children}</div>
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
};
