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

/**
 * A chip-based combobox built on cmdk (listbox, keyboard navigation,
 * `aria-activedescendant`) and Radix Popover (positioning, dismissal). It
 * exposes a controlled API shaped for `FilterCombobox`:
 *
 * - `open` is caller-owned; the popup only ever opens because the caller sets
 *   it, so the component reports the single event Radix can actually observe,
 *   `onDismiss` (escape / outside press).
 * - `value` is the committed chip tokens, rendered through `ComboboxValue`.
 *   Chips are removed via `onRemoveValue`; additions are driven by the caller.
 * - Dropdown rows are actions: each `ComboboxItem` invokes its own `onSelect`
 *   rather than mutating a selection array, so categories, value suggestions,
 *   and resource previews stay distinct concerns.
 * - `inputValue`/`onInputValueChange` is the free-text input, and
 *   `onItemHighlighted` reports the cmdk-highlighted row so callers can
 *   implement Tab completion.
 */

const ComboboxAnchorContext =
	createContext<RefObject<HTMLDivElement | null> | null>(null);

type ComboboxStateValue = {
	value: string[];
	inputValue: string;
	onInputValueChange?: (value: string) => void;
	onRemoveValue: (value: string) => void;
};

const ComboboxStateContext = createContext<ComboboxStateValue | null>(null);

function useComboboxState(): ComboboxStateValue {
	const context = useContext(ComboboxStateContext);
	if (!context) {
		throw new Error("Combobox components must be used within a <Combobox />");
	}
	return context;
}

// cmdk keeps a running count of the rows it renders; both the content wrapper
// and the list drive a `data-empty` styling group from it so `ComboboxEmpty`
// and empty-state padding can toggle via CSS.
function useComboboxIsEmpty(): boolean {
	return (useCommandState((state) => state.filtered.count) ?? 0) === 0;
}

type ComboboxProps = {
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

export function Combobox({
	open = false,
	onDismiss,
	value = [],
	onRemoveValue,
	inputValue = "",
	onInputValueChange,
	onItemHighlighted,
	label,
	children,
}: ComboboxProps) {
	const anchorRef = useRef<HTMLDivElement | null>(null);
	// cmdk only reports highlight changes through `onValueChange` when its value
	// is controlled, so track the highlighted row here and surface it to callers.
	const [highlightedValue, setHighlightedValue] = useState("");
	const onItemHighlightedRef = useRef(onItemHighlighted);
	onItemHighlightedRef.current = onItemHighlighted;
	const onDismissRef = useRef(onDismiss);
	onDismissRef.current = onDismiss;

	const state: ComboboxStateValue = {
		value,
		inputValue,
		onInputValueChange,
		onRemoveValue: (token) => onRemoveValue?.(token),
	};

	return (
		<ComboboxAnchorContext.Provider value={anchorRef}>
			<ComboboxStateContext.Provider value={state}>
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
					{/*
					 * There is no PopoverTrigger, so Radix never opens the popup on
					 * its own; opens flow from the caller through `open`. The only
					 * event Radix originates is a close request, which we forward as
					 * `onDismiss`.
					 */}
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
			</ComboboxStateContext.Provider>
		</ComboboxAnchorContext.Provider>
	);
}

type ComboboxValueProps = {
	children: (selected: string[]) => ReactNode;
};

export const ComboboxValue: FC<ComboboxValueProps> = ({ children }) => {
	const { value } = useComboboxState();
	return <>{children(value)}</>;
};

type ComboboxContentProps = ComponentPropsWithRef<typeof PopoverContent>;

export const ComboboxContent: FC<ComboboxContentProps> = ({
	className,
	align = "start",
	sideOffset = 6,
	...props
}) => {
	const anchorRef = useContext(ComboboxAnchorContext);
	const isEmpty = useComboboxIsEmpty();

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

type ComboboxListProps = ComponentPropsWithRef<typeof CommandPrimitive.List>;

export const ComboboxList: FC<ComboboxListProps> = ({
	className,
	...props
}) => {
	const isEmpty = useComboboxIsEmpty();

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

type ComboboxItemProps = ComponentPropsWithRef<typeof CommandPrimitive.Item>;

/**
 * A dropdown row. Rows are actions, not toggles: pass `onSelect` to run the
 * row's behavior (open a category, add a chip, navigate to a result). cmdk
 * calls `onSelect` on click and on Enter for the highlighted row.
 */
export const ComboboxItem: FC<ComboboxItemProps> = ({
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

type ComboboxGroupProps = ComponentPropsWithRef<typeof CommandPrimitive.Group>;

export const ComboboxGroup: FC<ComboboxGroupProps> = ({
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

type ComboboxLabelProps = ComponentPropsWithRef<"div">;

export const ComboboxLabel: FC<ComboboxLabelProps> = ({
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

type ComboboxEmptyProps = ComponentPropsWithRef<"div">;

export const ComboboxEmpty: FC<ComboboxEmptyProps> = ({
	className,
	...props
}) => {
	// Visibility is driven by the `data-empty` group set on `ComboboxContent`.
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

type ComboboxStatusProps = ComponentPropsWithRef<"div">;

export const ComboboxStatus: FC<ComboboxStatusProps> = ({
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

type ComboboxInputGroupProps = ComponentPropsWithRef<"div">;

export const ComboboxInputGroup: FC<ComboboxInputGroupProps> = ({
	className,
	...props
}) => {
	const anchorRef = useContext(ComboboxAnchorContext);

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

type ComboboxChipsProps = ComponentPropsWithRef<"div">;

export const ComboboxChips: FC<ComboboxChipsProps> = ({
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

type ComboboxChipProps = ComponentPropsWithRef<typeof Badge> & {
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
	const { onRemoveValue } = useComboboxState();
	const chipText =
		typeof children === "string" || typeof children === "number"
			? String(children)
			: undefined;
	const resolvedRemoveLabel =
		removeLabel ?? (chipText ? `Remove ${chipText}` : "Remove filter");

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
						if (chipText) {
							onRemoveValue(chipText);
						}
					}}
				>
					<XIcon aria-hidden className="size-icon-xs" />
				</button>
			)}
		</Badge>
	);
};

type ComboboxChipsInputProps = ComponentPropsWithRef<
	typeof CommandPrimitive.Input
>;

export const ComboboxChipsInput: FC<ComboboxChipsInputProps> = ({
	className,
	ref,
	...props
}) => {
	const { inputValue, onInputValueChange } = useComboboxState();

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
