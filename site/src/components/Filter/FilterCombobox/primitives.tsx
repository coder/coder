import { Command as CommandPrimitive, useCommandState } from "cmdk";
import { XIcon } from "lucide-react";
import {
	type ComponentPropsWithRef,
	createContext,
	type FC,
	type ReactNode,
	type RefObject,
	useContext,
	useRef,
	useState,
} from "react";
import { Badge } from "#/components/Badge/Badge";
import { InputGroup } from "#/components/InputGroup/InputGroup";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

// Primitive layer for `FilterCombobox`, built on cmdk (listbox, keyboard
// navigation) and Radix Popover (positioning, dismissal). These are prefixed
// `FilterCombobox*` and kept single-consumer to avoid shadowing the unrelated
// `components/Combobox` single-select primitives. They overlap cosmetically
// with `components/Command`; a future consolidation into a variant-driven
// `Command*` layer could remove the duplication.

const FilterComboboxAnchorContext =
	createContext<RefObject<HTMLDivElement | null> | null>(null);

type FilterComboboxStateValue = {
	value: string[];
	inputValue: string;
	onInputValueChange?: (value: string) => void;
	onRemoveValue?: (value: string) => void;
};

const FilterComboboxStateContext =
	createContext<FilterComboboxStateValue | null>(null);

function useFilterComboboxState(): FilterComboboxStateValue {
	const context = useContext(FilterComboboxStateContext);
	if (!context) {
		throw new Error(
			"FilterCombobox primitives must be used within a <FilterComboboxRoot />",
		);
	}
	return context;
}

// cmdk keeps a running count of the rows it renders; both the content wrapper
// and the list drive a `data-empty` styling group from it so
// `FilterComboboxEmpty` and empty-state padding can toggle via CSS.
function useFilterComboboxIsEmpty(): boolean {
	return (useCommandState((state) => state.filtered.count) ?? 0) === 0;
}

type FilterComboboxRootProps = {
	open?: boolean;
	/** Fired when Radix requests a close (escape / outside press). */
	onDismiss?: () => void;
	value?: string[];
	onRemoveValue?: (value: string) => void;
	inputValue?: string;
	onInputValueChange?: (value: string) => void;
	onItemHighlighted?: (value: string | undefined) => void;
	/** Accessible label for the input. cmdk wires it via `aria-labelledby`. */
	label?: string;
	children?: ReactNode;
};

/**
 * Controlled root for the filter combobox. `open` is caller-owned (there is no
 * PopoverTrigger), so the only event Radix originates is a close request,
 * surfaced as `onDismiss`. Dropdown rows are actions that fire their own
 * `onSelect`; `value` holds the committed chip tokens, removed via
 * `onRemoveValue`.
 *
 * Note: `FilterComboboxContent` renders in-flow (`disablePortal`, required so
 * cmdk can DOM-query its list). A consumer mounting this inside an
 * `overflow: hidden`/`auto` ancestor or a Radix `Dialog` may see the popup
 * clipped or z-index-inverted.
 */
export function FilterComboboxRoot({
	open = false,
	onDismiss,
	value = [],
	onRemoveValue,
	inputValue = "",
	onInputValueChange,
	onItemHighlighted,
	label,
	children,
}: FilterComboboxRootProps) {
	const anchorRef = useRef<HTMLDivElement | null>(null);
	// cmdk only reports highlight changes through `onValueChange` when its value
	// is controlled, so track the highlighted row here and surface it to callers.
	const [highlightedValue, setHighlightedValue] = useState("");
	const onItemHighlightedRef = useRef(onItemHighlighted);
	onItemHighlightedRef.current = onItemHighlighted;
	const onDismissRef = useRef(onDismiss);
	onDismissRef.current = onDismiss;

	const state: FilterComboboxStateValue = {
		value,
		inputValue,
		onInputValueChange,
		onRemoveValue,
	};

	return (
		<FilterComboboxAnchorContext value={anchorRef}>
			<FilterComboboxStateContext value={state}>
				<CommandPrimitive
					shouldFilter={false}
					loop
					label={label}
					className="flex w-full flex-col"
					value={highlightedValue}
					onValueChange={(highlighted) => {
						setHighlightedValue(highlighted);
						onItemHighlightedRef.current?.(highlighted || undefined);
					}}
				>
					{/* No PopoverTrigger: opens are caller-driven via `open`; Radix only
					    originates close requests, forwarded as `onDismiss`. */}
					<Popover
						open={open}
						onOpenChange={(nextOpen) => {
							if (!nextOpen) {
								onDismissRef.current?.();
							}
						}}
						modal={false}
					>
						{children}
					</Popover>
				</CommandPrimitive>
			</FilterComboboxStateContext>
		</FilterComboboxAnchorContext>
	);
}

type FilterComboboxValueProps = {
	children: (selected: string[]) => ReactNode;
};

export const FilterComboboxValue: FC<FilterComboboxValueProps> = ({
	children,
}) => {
	const { value } = useFilterComboboxState();
	return <>{children(value)}</>;
};

type FilterComboboxContentProps = ComponentPropsWithRef<typeof PopoverContent>;

export const FilterComboboxContent: FC<FilterComboboxContentProps> = ({
	className,
	align = "start",
	sideOffset = 6,
	...props
}) => {
	const anchorRef = useContext(FilterComboboxAnchorContext);
	const isEmpty = useFilterComboboxIsEmpty();

	return (
		<PopoverContent
			disablePortal
			align={align}
			sideOffset={sideOffset}
			data-empty={isEmpty ? "" : undefined}
			onOpenAutoFocus={(event) => event.preventDefault()}
			onInteractOutside={(event) => {
				const target = event.target as Node | null;
				if (target && anchorRef?.current?.contains(target)) {
					event.preventDefault();
				}
			}}
			className={cn(
				`group/combobox-content w-[var(--radix-popover-trigger-width)] overflow-hidden p-0
				max-h-[min(24rem,var(--radix-popper-available-height))]`,
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxListProps = ComponentPropsWithRef<
	typeof CommandPrimitive.List
>;

export const FilterComboboxList: FC<FilterComboboxListProps> = ({
	className,
	...props
}) => {
	const isEmpty = useFilterComboboxIsEmpty();

	return (
		<CommandPrimitive.List
			data-slot="combobox-list"
			data-empty={isEmpty ? "" : undefined}
			className={cn(
				"max-h-96 scroll-py-1 overflow-y-auto overscroll-contain p-1",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxItemProps = ComponentPropsWithRef<
	typeof CommandPrimitive.Item
>;

/**
 * A dropdown row. Rows are actions, not toggles: pass `onSelect` to run the
 * row's behavior (open a category, add a chip, navigate to a result). cmdk
 * calls `onSelect` on click and on Enter for the highlighted row.
 */
export const FilterComboboxItem: FC<FilterComboboxItemProps> = ({
	className,
	...props
}) => {
	return (
		<CommandPrimitive.Item
			data-slot="combobox-item"
			className={cn(
				`relative flex w-full cursor-default select-none items-center gap-2 rounded-sm
				px-2 py-1.5 text-sm text-content-secondary outline-none
				data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary
				data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50
				[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-icon-sm`,
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxGroupProps = ComponentPropsWithRef<
	typeof CommandPrimitive.Group
>;

export const FilterComboboxGroup: FC<FilterComboboxGroupProps> = ({
	className,
	...props
}) => {
	return (
		<CommandPrimitive.Group
			data-slot="combobox-group"
			className={cn(className)}
			{...props}
		/>
	);
};

type FilterComboboxLabelProps = ComponentPropsWithRef<"div">;

export const FilterComboboxLabel: FC<FilterComboboxLabelProps> = ({
	className,
	...props
}) => {
	return (
		<div
			data-slot="combobox-label"
			className={cn("px-2 py-1.5 text-xs text-content-secondary", className)}
			{...props}
		/>
	);
};

type FilterComboboxEmptyProps = ComponentPropsWithRef<"div">;

export const FilterComboboxEmpty: FC<FilterComboboxEmptyProps> = ({
	className,
	...props
}) => {
	// Visibility is driven by the `data-empty` group set on
	// `FilterComboboxContent`.
	return (
		<div
			data-slot="combobox-empty"
			className={cn(
				`hidden w-full justify-center py-6 text-center text-sm text-content-secondary
				group-data-[empty]/combobox-content:flex`,
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxStatusProps = ComponentPropsWithRef<"div">;

export const FilterComboboxStatus: FC<FilterComboboxStatusProps> = ({
	className,
	...props
}) => {
	return (
		<div
			data-slot="combobox-status"
			role="status"
			aria-live="polite"
			className={cn("sr-only", className)}
			{...props}
		/>
	);
};

type FilterComboboxInputGroupProps = ComponentPropsWithRef<"div">;

export const FilterComboboxInputGroup: FC<FilterComboboxInputGroupProps> = ({
	className,
	...props
}) => {
	const anchorRef = useContext(FilterComboboxAnchorContext);

	return (
		<PopoverAnchor asChild>
			<InputGroup
				ref={anchorRef ?? undefined}
				className={cn("h-auto min-h-10 w-full items-center", className)}
				{...props}
			/>
		</PopoverAnchor>
	);
};

type FilterComboboxChipsProps = ComponentPropsWithRef<"div">;

export const FilterComboboxChips: FC<FilterComboboxChipsProps> = ({
	className,
	...props
}) => {
	return (
		<div
			data-slot="combobox-chips"
			className={cn(
				"flex min-h-10 min-w-0 flex-1 flex-wrap content-center items-center gap-1 py-1",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxChipProps = ComponentPropsWithRef<typeof Badge> & {
	/**
	 * Token passed to `onRemoveValue` when the chip is removed. Decoupled from
	 * `children` so the chip can render richer content than a bare string.
	 * Falls back to the string/number children for convenience.
	 */
	value?: string;
	showRemove?: boolean;
	/** Accessible name for the remove control. Defaults to `Remove ${value}`. */
	removeLabel?: string;
};

export const FilterComboboxChip: FC<FilterComboboxChipProps> = ({
	className,
	children,
	value,
	showRemove = true,
	removeLabel,
	...props
}) => {
	const { onRemoveValue } = useFilterComboboxState();
	const childText =
		typeof children === "string" || typeof children === "number"
			? String(children)
			: undefined;
	const removeValue = value ?? childText;
	const resolvedRemoveLabel =
		removeLabel ?? (removeValue ? `Remove ${removeValue}` : "Remove filter");

	return (
		<Badge
			data-slot="combobox-chip"
			className={cn("font-medium text-content-primary", className)}
			{...props}
		>
			{children}
			{showRemove && (
				<button
					type="button"
					data-slot="combobox-chip-remove"
					aria-label={resolvedRemoveLabel}
					className={cn(
						`inline-flex size-4 shrink-0 items-center justify-center rounded-sm border-0
						bg-transparent p-0 text-content-secondary hover:text-content-primary`,
					)}
					onMouseDown={(event) => event.preventDefault()}
					onClick={(event) => {
						event.stopPropagation();
						if (removeValue) {
							onRemoveValue?.(removeValue);
						}
					}}
				>
					<XIcon aria-hidden className="size-icon-xs" />
				</button>
			)}
		</Badge>
	);
};

type FilterComboboxChipsInputProps = ComponentPropsWithRef<
	typeof CommandPrimitive.Input
>;

export const FilterComboboxChipsInput: FC<FilterComboboxChipsInputProps> = ({
	className,
	ref,
	...props
}) => {
	const { inputValue, onInputValueChange } = useFilterComboboxState();

	return (
		<CommandPrimitive.Input
			ref={ref}
			data-slot="combobox-chip-input"
			value={inputValue}
			onValueChange={(next) => onInputValueChange?.(next)}
			className={cn(
				`h-6 min-w-16 flex-1 border-0 bg-transparent p-0 text-sm font-medium
				text-content-primary outline-none placeholder:text-content-secondary`,
				className,
			)}
			{...props}
		/>
	);
};
