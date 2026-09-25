import { Command as CommandPrimitive } from "cmdk";
import { cn } from "cn";
import { XIcon } from "lucide-react";
import {
	type ComponentProps,
	createContext,
	type FC,
	type ReactNode,
	type Ref,
	type RefObject,
	useContext,
	useImperativeHandle,
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

// Primitive layer for `FilterCombobox`, built on cmdk (listbox, keyboard
// navigation) and Radix Popover (positioning, dismissal). These are prefixed
// `FilterCombobox*` and kept single-consumer to avoid shadowing the unrelated
// `components/Combobox` single-select primitives. They overlap cosmetically
// with `components/Command`; a future consolidation into a variant-driven
// `Command*` layer could remove the duplication.

/**
 * Height cap for the popup, and for each menu inside it when the caller makes
 * the popup `overflow-visible` so flyouts can extend past it, as
 * `FilterCombobox` does.
 */
// Bounded only by the viewport, so the menu scrolls only when it cannot fit.
export const menuMaxHeightClassName = "max-h-(--radix-popper-available-height)";

const FilterComboboxAnchorContext =
	createContext<RefObject<HTMLDivElement | null> | null>(null);

type FilterComboboxStateValue = {
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

/** Imperative handle to `FilterComboboxRoot`'s highlighted row. */
export type FilterComboboxHighlight = {
	get: () => string;
	set: (value: string) => void;
};

type FilterComboboxRootProps = {
	open?: boolean;
	/**
	 * Whether cmdk highlights the first row on its own. When false, a row is
	 * highlighted only by arrow keys, the pointer, or `highlightRef`.
	 */
	autoHighlight?: boolean;
	/** Fired when Radix requests a close (escape / outside press). */
	onDismiss?: () => void;
	onRemoveValue?: (value: string) => void;
	inputValue?: string;
	onInputValueChange?: (value: string) => void;
	/**
	 * The highlighted row lives here, so moving it re-renders only this root
	 * and the rows whose highlight changes, not the caller's option lists.
	 */
	highlightRef?: Ref<FilterComboboxHighlight>;
	onHighlightedValueChange?: (value: string) => void;
	/** Accessible label for the input. cmdk wires it via `aria-labelledby`. */
	label?: string;
	className?: string;
	children?: ReactNode;
};

/**
 * Controlled root for the filter combobox. `open` is caller-owned (there is no
 * PopoverTrigger), so the only event Radix originates is a close request,
 * surfaced as `onDismiss`. Dropdown rows are actions that fire their own
 * `onSelect`; committed chip removal is surfaced through `onRemoveValue`.
 *
 * Note: `FilterComboboxContent` renders in-flow (`disablePortal`, required so
 * cmdk can DOM-query its list). A consumer mounting this inside an
 * `overflow: hidden`/`auto` ancestor or a Radix `Dialog` may see the popup
 * clipped or z-index-inverted.
 */
export function FilterComboboxRoot({
	open = false,
	autoHighlight = true,
	onDismiss,
	onRemoveValue,
	inputValue = "",
	onInputValueChange,
	highlightRef,
	onHighlightedValueChange,
	label,
	className,
	children,
}: FilterComboboxRootProps) {
	const anchorRef = useRef<HTMLDivElement | null>(null);
	// cmdk only reports highlight changes when its value is controlled.
	const [highlightedValue, setHighlightedValue] = useState("");
	const highlightedValueRef = useRef("");
	// Set by arrow keys and pointer moves just before cmdk handles them, so
	// only those highlight a row while `autoHighlight` is off.
	const userNavigatingRef = useRef(false);
	useImperativeHandle(
		highlightRef,
		() => ({
			get: () => highlightedValueRef.current,
			set: (value) => {
				highlightedValueRef.current = value;
				setHighlightedValue(value);
			},
		}),
		[],
	);

	const state: FilterComboboxStateValue = {
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
					className={cn("flex w-full flex-col", className)}
					value={highlightedValue}
					onKeyDownCapture={(event) => {
						userNavigatingRef.current = [
							"ArrowUp",
							"ArrowDown",
							"Home",
							"End",
						].includes(event.key);
					}}
					onPointerMoveCapture={() => {
						userNavigatingRef.current = true;
					}}
					onValueChange={(value) => {
						const navigating = userNavigatingRef.current;
						userNavigatingRef.current = false;
						if (!autoHighlight && !navigating) {
							// cmdk has already stored the row it picked and only re-reads
							// a controlled value when it changes. It trims the value, so
							// switching between "" and " " resets it to nothing.
							setHighlightedValue((previous) => (previous === "" ? " " : ""));
							return;
						}
						highlightedValueRef.current = value;
						setHighlightedValue(value);
						onHighlightedValueChange?.(value);
					}}
				>
					{/* No PopoverTrigger: opens are caller-driven via `open`; Radix only
					    originates close requests, forwarded as `onDismiss`. */}
					<Popover
						open={open}
						onOpenChange={(nextOpen) => {
							if (!nextOpen) {
								onDismiss?.();
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

type FilterComboboxContentProps = ComponentProps<typeof PopoverContent>;

export const FilterComboboxContent: FC<FilterComboboxContentProps> = ({
	className,
	align = "start",
	sideOffset = 6,
	...props
}) => {
	const anchorRef = useContext(FilterComboboxAnchorContext);
	return (
		<PopoverContent
			disablePortal
			align={align}
			sideOffset={sideOffset}
			onOpenAutoFocus={(event) => event.preventDefault()}
			onInteractOutside={(event) => {
				if (
					event.target instanceof Node &&
					anchorRef?.current?.contains(event.target)
				) {
					event.preventDefault();
				}
			}}
			className={cn(
				menuMaxHeightClassName,
				"flex w-(--radix-popover-trigger-width) flex-col overflow-y-hidden p-0",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxListProps = ComponentProps<typeof CommandPrimitive.List>;

export const FilterComboboxList: FC<FilterComboboxListProps> = ({
	className,
	...props
}) => {
	return (
		<CommandPrimitive.List
			data-slot="combobox-list"
			className={cn(
				"min-h-0 scroll-py-1 overflow-y-auto overscroll-contain p-1",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxItemProps = ComponentProps<typeof CommandPrimitive.Item>;

/**
 * A dropdown row. Rows are actions, not toggles: cmdk calls `onSelect` on
 * click and on Enter for the highlighted row.
 */
export const FilterComboboxItem: FC<FilterComboboxItemProps> = ({
	className,
	...props
}) => {
	return (
		<CommandPrimitive.Item
			data-slot="combobox-item"
			className={cn(
				"relative flex w-full cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm font-normal text-content-secondary outline-hidden data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-icon-sm",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxGroupProps = ComponentProps<typeof CommandPrimitive.Group>;

export const FilterComboboxGroup: FC<FilterComboboxGroupProps> = ({
	className,
	...props
}) => {
	return (
		<CommandPrimitive.Group
			data-slot="combobox-group"
			className={cn("group/combobox-group", className)}
			{...props}
		/>
	);
};

type FilterComboboxLabelProps = ComponentProps<"div">;

export const FilterComboboxLabel: FC<FilterComboboxLabelProps> = ({
	className,
	...props
}) => {
	return (
		<div
			data-slot="combobox-label"
			// The first header relies on the list's own padding for its top space.
			className={cn(
				"px-2 pt-4 pb-2 text-xs text-content-secondary group-first/combobox-group:pt-0",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxStatusProps = ComponentProps<"div">;

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

type FilterComboboxInputGroupProps = ComponentProps<"div">;

export const FilterComboboxInputGroup: FC<FilterComboboxInputGroupProps> = ({
	className,
	...props
}) => {
	const anchorRef = useContext(FilterComboboxAnchorContext);

	return (
		<PopoverAnchor asChild>
			<InputGroup
				ref={anchorRef ?? undefined}
				className={cn("h-auto min-h-10 w-full items-start", className)}
				{...props}
			/>
		</PopoverAnchor>
	);
};

type FilterComboboxChipsProps = ComponentProps<"div">;

export const FilterComboboxChips: FC<FilterComboboxChipsProps> = ({
	className,
	...props
}) => {
	return (
		<div
			data-slot="combobox-chips"
			className={cn(
				"flex min-h-9.5 min-w-0 flex-1 flex-wrap content-center items-center gap-1 py-1.25 pr-2",
				className,
			)}
			{...props}
		/>
	);
};

type FilterComboboxChipProps = ComponentProps<typeof Badge> & {
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
	// The button unmounts with its chip, so keyboard removal moves focus to the
	// search input instead of the page body.
	const remove = (button: HTMLElement, fromKeyboard: boolean) => {
		if (fromKeyboard) {
			button
				.closest('[data-slot="combobox-chips"]')
				?.querySelector("input")
				?.focus();
		}
		if (removeValue) {
			onRemoveValue?.(removeValue);
		}
	};

	return (
		<Badge
			data-slot="combobox-chip"
			svgSize="sm"
			className={cn(
				"group/chip pl-2 font-medium text-content-secondary hover:text-content-primary",
				className,
			)}
			{...props}
		>
			{children}
			{showRemove && (
				<button
					type="button"
					data-slot="combobox-chip-remove"
					aria-label={resolvedRemoveLabel}
					className={cn(
						"inline-flex shrink-0 items-center justify-center rounded-sm border-0 bg-transparent p-0",
					)}
					onMouseDown={(event) => event.preventDefault()}
					// The cmdk root cancels Enter, so it is handled here.
					onKeyDown={(event) => {
						if (event.key !== "Enter") {
							return;
						}
						event.preventDefault();
						event.stopPropagation();
						remove(event.currentTarget, true);
					}}
					onClick={(event) => {
						event.stopPropagation();
						remove(event.currentTarget, event.detail === 0);
					}}
				>
					<XIcon aria-hidden />
				</button>
			)}
		</Badge>
	);
};

type FilterComboboxChipsInputProps = ComponentProps<
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
			// Content-sized so an empty input fits in the space after the last chip
			// instead of forcing a new row; `size={1}` is the fallback intrinsic
			// width. Height matches a chip so the box stays the same height with or
			// without chips.
			size={1}
			className={cn(
				"h-7 min-w-1 flex-auto field-sizing-content border-0 bg-transparent p-0 text-sm font-medium text-content-primary outline-hidden placeholder:text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};
