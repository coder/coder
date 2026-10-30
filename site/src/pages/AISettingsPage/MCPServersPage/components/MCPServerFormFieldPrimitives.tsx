import { cn } from "cn";
import { ChevronDownIcon, ChevronRightIcon, InfoIcon } from "lucide-react";
import { useId } from "react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Label } from "#/components/Label/Label";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

const RequiredMark = () => (
	<span className="text-xs font-bold text-content-destructive">*</span>
);

export const Field: React.FC<{
	label: React.ReactNode;
	htmlFor?: string;
	required?: boolean;
	children: React.ReactNode;
	description?: React.ReactNode;
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

type SwitchFieldProps = {
	label: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
	disabled: boolean;
	/** Explains the setting in a tooltip next to its label. */
	tooltip?: string;
	/** Always visible under the label, and read out with the switch. */
	description?: string;
};

/** A switch with a label that toggles it. */
export const SwitchField: React.FC<SwitchFieldProps> = ({
	label,
	checked,
	onCheckedChange,
	disabled,
	tooltip,
	description,
}) => {
	const id = useId();
	const descriptionId = useId();
	return (
		<div className="flex flex-col gap-1">
			<div className="flex items-center gap-3">
				<Switch
					id={id}
					checked={checked}
					onCheckedChange={onCheckedChange}
					disabled={disabled}
					aria-describedby={description ? descriptionId : undefined}
				/>
				<Label htmlFor={id} className="cursor-pointer font-normal">
					{label}
				</Label>
				{tooltip && (
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								aria-label={`About ${label}`}
								className="flex cursor-help border-0 bg-transparent p-0 text-content-secondary"
							>
								<InfoIcon className="size-3" />
							</button>
						</TooltipTrigger>
						<TooltipContent side="top" className="max-w-65">
							{tooltip}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
			{description && (
				<p id={descriptionId} className="m-0 text-xs text-content-secondary">
					{description}
				</p>
			)}
		</div>
	);
};

export const CollapsibleSection: React.FC<{
	title: string;
	description: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	className?: string;
	contentClassName?: string;
	children: React.ReactNode;
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
					<ChevronDownIcon className="mt-1 size-4 shrink-0 text-content-primary" />
				) : (
					<ChevronRightIcon className="mt-1 size-4 shrink-0 text-content-primary" />
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
