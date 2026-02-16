/**
 * Copied from shadc/ui on 13/01/2025
 * @see {@link https://ui.shadcn.com/docs/components/select}
 */
import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown, ChevronUp } from "lucide-react";
import { cn } from "utils/cn";

export const Select = SelectPrimitive.Root;

export const SelectGroup = SelectPrimitive.Group;

export const SelectValue = SelectPrimitive.Value;

export type SelectTriggerProps = React.ComponentPropsWithRef<
	typeof SelectPrimitive.Trigger
>;

export const SelectTrigger: React.FC<SelectTriggerProps> = ({
	className,
	children,
	...props
}) => (
	<SelectPrimitive.Trigger
		className={cn(
			`flex h-10 w-full font-medium items-center justify-between whitespace-nowrap rounded-md
			border border-border border-solid bg-transparent px-3 py-2 text-sm shadow-sm
			ring-offset-background text-content-secondary placeholder:text-content-secondary focus:outline-none,
			focus:ring-2 focus:ring-content-link disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1
			focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link`,
			className,
		)}
		{...props}
	>
		{children}
		<SelectPrimitive.Icon asChild>
			<ChevronDown className="cursor-pointer text-content-secondary hover:text-content-primary p-0.5" />
		</SelectPrimitive.Icon>
	</SelectPrimitive.Trigger>
);

const SelectScrollUpButton: React.FC<
	React.ComponentPropsWithRef<typeof SelectPrimitive.ScrollUpButton>
> = ({ className, ...props }) => (
	<SelectPrimitive.ScrollUpButton
		className={cn(
			"flex cursor-default items-center justify-center py-1",
			className,
		)}
		{...props}
	>
		<ChevronUp className="size-icon-sm" />
	</SelectPrimitive.ScrollUpButton>
);

const SelectScrollDownButton: React.FC<
	React.ComponentPropsWithRef<typeof SelectPrimitive.ScrollDownButton>
> = ({ className, ...props }) => {
	return (
		<SelectPrimitive.ScrollDownButton
			className={cn(
				"flex cursor-default items-center justify-center py-1",
				className,
			)}
			{...props}
		>
			<ChevronDown className="size-icon-sm cursor-pointer text-content-secondary hover:text-content-primary" />
		</SelectPrimitive.ScrollDownButton>
	);
};

export const SelectContent: React.FC<
	React.ComponentPropsWithRef<typeof SelectPrimitive.Content>
> = ({ className, children, position = "popper", ...props }) => (
	<SelectPrimitive.Portal>
		<SelectPrimitive.Content
			className={cn(
				"relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-md border ",
				"border-border border-solid bg-surface-primary text-content-primary shadow-md ",
				"data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 ",
				"data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 ",
				"data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 ",
				"data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
				position === "popper" &&
					"data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 ",
				"data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1",
				className,
			)}
			position={position}
			{...props}
		>
			<SelectScrollUpButton />
			<SelectPrimitive.Viewport
				className={cn(
					"p-1",
					position === "popper" &&
						"h-[var(--radix-select-trigger-height)] w-full min-w-[var(--radix-select-trigger-width)]",
				)}
			>
				{children}
			</SelectPrimitive.Viewport>
			<SelectScrollDownButton />
		</SelectPrimitive.Content>
	</SelectPrimitive.Portal>
);

export const SelectLabel: React.FC<
	React.ComponentPropsWithRef<typeof SelectPrimitive.Label>
> = ({ className, ...props }) => {
	return (
		<SelectPrimitive.Label
			className={cn("px-2 py-1.5 text-sm font-semibold", className)}
			{...props}
		/>
	);
};

export const SelectItem: React.FC<
	React.ComponentPropsWithRef<typeof SelectPrimitive.Item>
> = ({ className, children, ...props }) => (
	<SelectPrimitive.Item
		className={cn(
			"relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 ",
			"pl-2 pr-8 text-sm text-content-secondary outline-none focus:bg-surface-secondary ",
			"focus:text-content-primary data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
			className,
		)}
		{...props}
	>
		<span className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center justify-center">
			<SelectPrimitive.ItemIndicator className="size-icon-sm">
				<Check className="size-icon-sm" />
			</SelectPrimitive.ItemIndicator>
		</span>
		<SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
	</SelectPrimitive.Item>
);
