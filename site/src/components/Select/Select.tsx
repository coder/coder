/**
 * Copied from shadc/ui on 13/01/2025
 * @see {@link https://ui.shadcn.com/docs/components/select}
 */
import * as SelectPrimitive from "@radix-ui/react-select";
import { Check, ChevronDown, ChevronUp } from "lucide-react";
import type * as React from "react";
import { cn } from "utils/cn";

export const Select = SelectPrimitive.Root;

export const SelectGroup = SelectPrimitive.Group;

export const SelectValue = SelectPrimitive.Value;

export type SelectTriggerProps = React.ComponentPropsWithRef<
	typeof SelectPrimitive.Trigger
>;

export const SelectTrigger = ({
	className,
	children,
	id,
	ref,
	...props
}: SelectTriggerProps) => (
	<SelectPrimitive.Trigger
		ref={ref}
		id={id}
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
			<ChevronDown className="size-icon-sm cursor-pointer text-content-secondary hover:text-content-primary" />
		</SelectPrimitive.Icon>
	</SelectPrimitive.Trigger>
);
SelectTrigger.displayName = SelectPrimitive.Trigger.displayName;

const SelectScrollUpButton = ({
	className,
	ref,
	...props
}: React.ComponentPropsWithRef<typeof SelectPrimitive.ScrollUpButton>) => (
	<SelectPrimitive.ScrollUpButton
		ref={ref}
		className={cn(
			"flex cursor-default items-center justify-center py-1",
			className,
		)}
		{...props}
	>
		<ChevronUp />
	</SelectPrimitive.ScrollUpButton>
);
SelectScrollUpButton.displayName = SelectPrimitive.ScrollUpButton.displayName;

const SelectScrollDownButton = ({
	className,
	ref,
	...props
}: React.ComponentPropsWithRef<typeof SelectPrimitive.ScrollDownButton>) => (
	<SelectPrimitive.ScrollDownButton
		ref={ref}
		className={cn(
			"flex cursor-default items-center justify-center py-1",
			className,
		)}
		{...props}
	>
		<ChevronDown />
	</SelectPrimitive.ScrollDownButton>
);
SelectScrollDownButton.displayName =
	SelectPrimitive.ScrollDownButton.displayName;

export const SelectContent = ({
	className,
	children,
	position = "popper",
	ref,
	...props
}: React.ComponentPropsWithRef<typeof SelectPrimitive.Content>) => (
	<SelectPrimitive.Portal>
		<SelectPrimitive.Content
			ref={ref}
			className={cn(
				"relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-md border ",
				"border-border border-solid bg-surface-primary text-content-primary shadow-md ",
				"data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 ",
				"data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 ",
				"data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 ",
				"data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
				position === "popper" &&
					"data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1",
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
SelectContent.displayName = SelectPrimitive.Content.displayName;

export const SelectLabel = ({
	className,
	ref,
	...props
}: React.ComponentPropsWithRef<typeof SelectPrimitive.Label>) => (
	<SelectPrimitive.Label
		ref={ref}
		className={cn("px-2 py-1.5 text-sm font-semibold", className)}
		{...props}
	/>
);
SelectLabel.displayName = SelectPrimitive.Label.displayName;

export const SelectItem = ({
	className,
	children,
	ref,
	...props
}: React.ComponentPropsWithRef<typeof SelectPrimitive.Item>) => (
	<SelectPrimitive.Item
		ref={ref}
		className={cn(
			"relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 ",
			"pl-2 pr-8 text-sm text-content-secondary outline-none focus:bg-surface-secondary ",
			"focus:text-content-primary data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
			className,
		)}
		{...props}
	>
		<span className="absolute right-2 flex h-3.5 w-3.5 items-center justify-center">
			<SelectPrimitive.ItemIndicator>
				<Check className="h-4 w-4" />
			</SelectPrimitive.ItemIndicator>
		</span>
		<SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
	</SelectPrimitive.Item>
);
SelectItem.displayName = SelectPrimitive.Item.displayName;

const SelectSeparator = ({
	className,
	ref,
	...props
}: React.ComponentPropsWithRef<typeof SelectPrimitive.Separator>) => (
	<SelectPrimitive.Separator
		ref={ref}
		className={cn("-mx-1 my-1 h-px bg-muted", className)}
		{...props}
	/>
);
SelectSeparator.displayName = SelectPrimitive.Separator.displayName;
