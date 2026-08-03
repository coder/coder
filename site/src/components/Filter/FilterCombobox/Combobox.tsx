import { Combobox as ComboboxPrimitive } from "@base-ui/react";
import { CheckIcon, XIcon } from "lucide-react";
import {
	type ComponentPropsWithRef,
	createContext,
	type FC,
	type RefObject,
	useContext,
	useRef,
} from "react";
import { Badge } from "#/components/Badge/Badge";
import { InputGroup } from "#/components/InputGroup/InputGroup";
import { cn } from "#/utils/cn";

const ComboboxAnchorContext =
	createContext<RefObject<HTMLDivElement | null> | null>(null);

type ComboboxProps<
	Value = string,
	Multiple extends boolean | undefined = true,
> = Omit<ComboboxPrimitive.Root.Props<Value, Multiple>, "autoHighlight"> & {
	/**
	 * Whether the first matching item is highlighted automatically.
	 * Use `"always"` to highlight the first item as soon as the list opens,
	 * including when options load asynchronously.
	 */
	autoHighlight?: boolean | "always";
};

export function Combobox<
	Value = string,
	Multiple extends boolean | undefined = true,
>({ autoHighlight, ...props }: ComboboxProps<Value, Multiple>) {
	const anchorRef = useRef<HTMLDivElement | null>(null);

	return (
		<ComboboxAnchorContext.Provider value={anchorRef}>
			<ComboboxPrimitive.Root
				{...props}
				// Base UI runtime accepts "always"; public Root types only allow boolean.
				autoHighlight={autoHighlight as boolean | undefined}
			/>
		</ComboboxAnchorContext.Provider>
	);
}

type ComboboxValueProps = ComboboxPrimitive.Value.Props;

export const ComboboxValue: FC<ComboboxValueProps> = (props) => {
	return <ComboboxPrimitive.Value data-slot="combobox-value" {...props} />;
};

type ComboboxContentProps = ComboboxPrimitive.Popup.Props &
	Pick<
		ComboboxPrimitive.Positioner.Props,
		"side" | "align" | "sideOffset" | "alignOffset" | "anchor"
	>;

export const ComboboxContent: FC<ComboboxContentProps> = ({
	className,
	side = "bottom",
	sideOffset = 6,
	align = "start",
	alignOffset = 0,
	anchor,
	...props
}) => {
	const defaultAnchor = useContext(ComboboxAnchorContext);

	return (
		<ComboboxPrimitive.Portal>
			<ComboboxPrimitive.Positioner
				side={side}
				sideOffset={sideOffset}
				align={align}
				alignOffset={alignOffset}
				anchor={anchor ?? defaultAnchor}
				className="isolate z-50"
			>
				<ComboboxPrimitive.Popup
					data-slot="combobox-content"
					className={cn(
						`group/combobox-content relative z-50 max-h-[min(24rem,var(--available-height))]
						w-[var(--anchor-width)] min-w-[var(--anchor-width)] overflow-hidden rounded-md
						border border-solid border-border bg-surface-primary text-content-primary shadow-md
						origin-[var(--transform-origin)]
						data-[open]:animate-in data-[open]:fade-in-0 data-[open]:zoom-in-95
						data-[closed]:animate-out data-[closed]:fade-out-0 data-[closed]:zoom-out-95
						data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2
						data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2`,
						className,
					)}
					{...props}
				/>
			</ComboboxPrimitive.Positioner>
		</ComboboxPrimitive.Portal>
	);
};

type ComboboxListProps = ComboboxPrimitive.List.Props;

export const ComboboxList: FC<ComboboxListProps> = ({
	className,
	...props
}) => {
	return (
		<ComboboxPrimitive.List
			data-slot="combobox-list"
			className={cn(
				"max-h-96 scroll-py-1 overflow-y-auto overscroll-contain p-1 data-[empty]:p-0",
				className,
			)}
			{...props}
		/>
	);
};

type ComboboxItemProps = ComboboxPrimitive.Item.Props & {
	showIndicator?: boolean;
};

export const ComboboxItem: FC<ComboboxItemProps> = ({
	className,
	children,
	showIndicator = true,
	...props
}) => {
	return (
		<ComboboxPrimitive.Item
			data-slot="combobox-item"
			className={cn(
				`relative flex w-full cursor-default select-none items-center gap-2 rounded-sm
				py-1.5 pr-8 pl-2 text-sm text-content-secondary outline-none
				data-[highlighted]:bg-surface-secondary data-[highlighted]:text-content-primary
				data-[disabled]:pointer-events-none data-[disabled]:opacity-50
				[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-icon-sm`,
				!showIndicator && "pr-2",
				className,
			)}
			{...props}
		>
			{children}
			{showIndicator && (
				<ComboboxPrimitive.ItemIndicator
					render={
						<span className="pointer-events-none absolute right-2 flex size-icon-sm items-center justify-center">
							<CheckIcon className="pointer-events-none size-icon-sm" />
						</span>
					}
				/>
			)}
		</ComboboxPrimitive.Item>
	);
};

type ComboboxGroupProps = ComboboxPrimitive.Group.Props;

export const ComboboxGroup: FC<ComboboxGroupProps> = ({
	className,
	...props
}) => {
	return (
		<ComboboxPrimitive.Group
			data-slot="combobox-group"
			className={cn(className)}
			{...props}
		/>
	);
};

type ComboboxLabelProps = ComboboxPrimitive.GroupLabel.Props;

export const ComboboxLabel: FC<ComboboxLabelProps> = ({
	className,
	...props
}) => {
	return (
		<ComboboxPrimitive.GroupLabel
			data-slot="combobox-label"
			className={cn("px-2 py-1.5 text-xs text-content-secondary", className)}
			{...props}
		/>
	);
};

type ComboboxEmptyProps = ComboboxPrimitive.Empty.Props;

export const ComboboxEmpty: FC<ComboboxEmptyProps> = ({
	className,
	...props
}) => {
	return (
		<ComboboxPrimitive.Empty
			data-slot="combobox-empty"
			className={cn(
				"hidden w-full justify-center py-6 text-center text-sm text-content-secondary group-data-[empty]/combobox-content:flex",
				className,
			)}
			{...props}
		/>
	);
};

type ComboboxStatusProps = ComboboxPrimitive.Status.Props;

export const ComboboxStatus: FC<ComboboxStatusProps> = ({
	className,
	...props
}) => {
	return (
		<ComboboxPrimitive.Status
			data-slot="combobox-status"
			className={cn("sr-only", className)}
			{...props}
		/>
	);
};

type ComboboxInputGroupProps = ComponentPropsWithRef<"div">;

export const ComboboxInputGroup: FC<ComboboxInputGroupProps> = ({
	className,
	...props
}) => {
	const anchorRef = useContext(ComboboxAnchorContext);

	return (
		<InputGroup
			ref={anchorRef}
			className={cn("h-auto min-h-10 w-full", className)}
			{...props}
		/>
	);
};

type ComboboxChipsProps = ComponentPropsWithRef<
	typeof ComboboxPrimitive.Chips
> &
	ComboboxPrimitive.Chips.Props;

export const ComboboxChips: FC<ComboboxChipsProps> = ({
	className,
	...props
}) => {
	return (
		<ComboboxPrimitive.Chips
			data-slot="combobox-chips"
			className={cn(
				`flex min-h-8 min-w-0 flex-1 flex-wrap items-center gap-1 self-stretch
				border-0 bg-transparent py-1.5 shadow-none focus-within:ring-0`,
				className,
			)}
			{...props}
		/>
	);
};

type ComboboxChipProps = ComboboxPrimitive.Chip.Props & {
	showRemove?: boolean;
	/** Accessible name for the remove control. Defaults to `Remove ${label}`. */
	removeLabel?: string;
};

export const ComboboxChip: FC<ComboboxChipProps> = ({
	className,
	children,
	showRemove = true,
	removeLabel,
	...props
}) => {
	const chipText =
		typeof children === "string" || typeof children === "number"
			? String(children)
			: undefined;
	const resolvedRemoveLabel =
		removeLabel ?? (chipText ? `Remove ${chipText}` : "Remove filter");

	return (
		<ComboboxPrimitive.Chip
			data-slot="combobox-chip"
			render={<Badge className="font-medium text-content-primary" />}
			className={cn(
				"outline-none focus-visible:ring-2 focus-visible:ring-content-link",
				"data-[highlighted]:bg-surface-tertiary data-[highlighted]:ring-2",
				"data-[highlighted]:ring-content-link",
				"has-[:disabled]:pointer-events-none",
				"has-[:disabled]:cursor-not-allowed",
				"has-[:disabled]:opacity-50",
				className,
			)}
			{...props}
		>
			{children}
			{showRemove && (
				<ComboboxPrimitive.ChipRemove
					data-slot="combobox-chip-remove"
					className={cn(
						`inline-flex size-4 shrink-0 items-center justify-center rounded-sm border-0
						bg-transparent p-0 text-content-secondary hover:text-content-primary`,
					)}
					aria-label={resolvedRemoveLabel}
				>
					<XIcon aria-hidden className="size-icon-xs" />
				</ComboboxPrimitive.ChipRemove>
			)}
		</ComboboxPrimitive.Chip>
	);
};

type ComboboxChipsInputProps = ComboboxPrimitive.Input.Props;

export const ComboboxChipsInput: FC<ComboboxChipsInputProps> = ({
	className,
	ref,
	...props
}) => {
	return (
		<ComboboxPrimitive.Input
			ref={ref}
			data-slot="combobox-chip-input"
			className={cn(
				`h-6 min-w-24 flex-1 border-0 bg-transparent p-0 text-sm font-medium
				text-content-primary outline-none placeholder:text-content-secondary`,
				className,
			)}
			{...props}
		/>
	);
};
