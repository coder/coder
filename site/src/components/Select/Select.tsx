import { Select as SelectPrimitive } from "@base-ui/react/select";
import { cn } from "cn";
import {
	CheckIcon,
	ChevronUpIcon,
	ChevronDownIcon as LucideChevronDown,
} from "lucide-react";
import { Children, isValidElement } from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";

function getSelectItems(
	children: React.ReactNode,
): { value: string; label: React.ReactNode }[] {
	return Children.toArray(children).flatMap((child) => {
		if (!isValidElement<{ value?: string; children?: React.ReactNode }>(child))
			return [];
		if (child.type === SelectItem && child.props.value !== undefined) {
			return [{ value: child.props.value, label: child.props.children }];
		}
		return getSelectItems(child.props.children);
	});
}

type SelectProps<Value extends string> = Omit<
	SelectPrimitive.Root.Props<Value>,
	"onValueChange"
> & {
	onValueChange?: (value: Value) => void;
};

/** Maps item labels for closed selects and keeps string values for form consumers. */
export const Select = <Value extends string>({
	children,
	items,
	value,
	defaultValue,
	onValueChange,
	...props
}: SelectProps<Value>) => (
	<SelectPrimitive.Root
		items={items ?? getSelectItems(children)}
		value={value === "" ? null : value}
		defaultValue={defaultValue === "" ? null : defaultValue}
		onValueChange={(value) => {
			if (value !== null) onValueChange?.(value);
		}}
		{...props}
	>
		{children}
	</SelectPrimitive.Root>
);

export const SelectGroup = SelectPrimitive.Group;

export const SelectValue = SelectPrimitive.Value;

type SelectTriggerProps = Omit<
	React.ComponentProps<typeof SelectPrimitive.Trigger>,
	"className"
> & { className?: string };

export const SelectTrigger: React.FC<SelectTriggerProps> = ({
	className,
	children,
	...props
}) => (
	<SelectPrimitive.Trigger
		className={cn(
			`flex h-10 w-full font-medium items-center justify-between whitespace-nowrap rounded-md
			border border-border border-solid bg-transparent px-3 py-2 text-sm shadow-xs
			ring-offset-background text-content-secondary placeholder:text-content-secondary
			disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1
			focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link group`,
			className,
		)}
		{...props}
	>
		{children}
		<SelectPrimitive.Icon
			render={
				<ChevronDownIcon className="size-icon-sm cursor-pointer text-content-secondary transition-colors hover:text-content-primary group-hover:text-content-primary" />
			}
		></SelectPrimitive.Icon>
	</SelectPrimitive.Trigger>
);

type SelectScrollUpButtonProps = Omit<
	React.ComponentProps<typeof SelectPrimitive.ScrollUpArrow>,
	"className"
> & { className?: string };

const SelectScrollUpButton: React.FC<SelectScrollUpButtonProps> = ({
	className,
	...props
}) => (
	<SelectPrimitive.ScrollUpArrow
		className={cn(
			"flex cursor-default items-center justify-center py-1",
			className,
		)}
		{...props}
	>
		<ChevronUpIcon className="size-icon-sm" />
	</SelectPrimitive.ScrollUpArrow>
);

type SelectScrollDownButtonProps = Omit<
	React.ComponentProps<typeof SelectPrimitive.ScrollDownArrow>,
	"className"
> & { className?: string };

const SelectScrollDownButton: React.FC<SelectScrollDownButtonProps> = ({
	className,
	...props
}) => {
	return (
		<SelectPrimitive.ScrollDownArrow
			className={cn(
				"flex cursor-default items-center justify-center py-1",
				className,
			)}
			{...props}
		>
			<LucideChevronDown className="size-icon-sm cursor-pointer text-content-secondary hover:text-content-primary" />
		</SelectPrimitive.ScrollDownArrow>
	);
};

type SelectContentProps = Omit<SelectPrimitive.Popup.Props, "className"> & {
	className?: string;
} & Pick<
		SelectPrimitive.Positioner.Props,
		"side" | "sideOffset" | "align" | "alignOffset"
	>;

export const SelectContent: React.FC<SelectContentProps> = ({
	className,
	children,
	side = "bottom",
	sideOffset = 4,
	align = "start",
	alignOffset,
	...props
}) => (
	<SelectPrimitive.Portal>
		<SelectPrimitive.Positioner
			side={side}
			sideOffset={sideOffset}
			align={align}
			alignOffset={alignOffset}
			alignItemWithTrigger={false}
			className="z-50 outline-hidden"
		>
			<SelectPrimitive.Popup
				className={cn(
					"relative min-w-32 max-h-[min(24rem,var(--available-height))] min-w-(--anchor-width) overflow-hidden rounded-md border border-solid border-border bg-surface-primary text-content-primary shadow-md",
					"origin-(--transform-origin) ease-out-strong",
					"data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-95 data-open:zoom-in-95",
					"data-open:data-[side=bottom]:slide-in-from-top-2 data-open:data-[side=left]:slide-in-from-right-2 data-open:data-[side=right]:slide-in-from-left-2 data-open:data-[side=top]:slide-in-from-bottom-2",
					className,
				)}
				{...props}
			>
				<SelectScrollUpButton />
				<SelectPrimitive.List className="max-h-[min(24rem,var(--available-height))] overflow-y-auto p-1">
					{children}
				</SelectPrimitive.List>
				<SelectScrollDownButton />
			</SelectPrimitive.Popup>
		</SelectPrimitive.Positioner>
	</SelectPrimitive.Portal>
);

type SelectLabelProps = Omit<
	React.ComponentProps<typeof SelectPrimitive.GroupLabel>,
	"className"
> & {
	className?: string;
};

export const SelectLabel: React.FC<SelectLabelProps> = ({
	className,
	...props
}) => {
	return (
		<SelectPrimitive.GroupLabel
			className={cn("px-2 py-1.5 text-sm font-semibold", className)}
			{...props}
		/>
	);
};

type SelectItemProps = Omit<
	React.ComponentProps<typeof SelectPrimitive.Item>,
	"className"
> & {
	className?: string;
};

export const SelectItem: React.FC<SelectItemProps> = ({
	className,
	children,
	...props
}) => (
	<SelectPrimitive.Item
		className={cn(
			"relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 ",
			"pl-2 pr-8 text-sm text-content-secondary outline-hidden data-highlighted:bg-surface-secondary ",
			"data-highlighted:text-content-primary data-disabled:pointer-events-none data-disabled:opacity-50",
			"focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
			className,
		)}
		{...props}
	>
		<span className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center justify-center">
			<SelectPrimitive.ItemIndicator className="size-icon-sm">
				<CheckIcon className="size-icon-sm" />
			</SelectPrimitive.ItemIndicator>
		</span>
		<SelectPrimitive.ItemText>{children}</SelectPrimitive.ItemText>
	</SelectPrimitive.Item>
);
