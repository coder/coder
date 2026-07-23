import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Label } from "#/components/Label/Label";
import { cn } from "#/utils/cn";

const RequiredMark = () => (
	<span className="text-xs font-bold text-content-destructive">*</span>
);

export const Field: FC<{
	label: ReactNode;
	htmlFor?: string;
	required?: boolean;
	children: ReactNode;
	description?: ReactNode;
	className?: string;
}> = ({ label, htmlFor, required, children, description, className }) => {
	return (
		<div className={cn("grid gap-1.5", className)}>
			<Label
				htmlFor={htmlFor}
				className="flex items-center gap-1 leading-6 text-content-primary"
			>
				{label}
				{required && <RequiredMark />}
			</Label>
			{description && (
				<p className="m-0 text-xs text-content-secondary">{description}</p>
			)}
			{children}
		</div>
	);
};

export const CollapsibleSection: FC<{
	title: string;
	description: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	className?: string;
	contentClassName?: string;
	children: ReactNode;
}> = ({
	title,
	description,
	open,
	onOpenChange,
	className,
	contentClassName,
	children,
}) => {
	return (
		<Collapsible
			open={open}
			onOpenChange={onOpenChange}
			className={cn("p-4", className)}
		>
			<CollapsibleTrigger className="flex w-full cursor-pointer items-start gap-2 border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary">
				{open ? (
					<ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-content-primary" />
				) : (
					<ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-content-primary" />
				)}
				<div>
					<h3 className="m-0 text-sm font-medium text-content-primary">
						{title}
					</h3>
					<p className="m-0 text-sm text-content-secondary">{description}</p>
				</div>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<div className={contentClassName}>{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};
