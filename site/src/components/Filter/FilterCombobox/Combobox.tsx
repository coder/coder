import { Command as CommandPrimitive, useCommandState } from "cmdk";
import { CheckIcon, XIcon } from "lucide-react";
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
 * A chip-based multi-select combobox built on cmdk (listbox, keyboard
 * navigation, `aria-activedescendant`) and Radix Popover (positioning,
 * dismissal). It exposes a controlled API shaped for `FilterCombobox`:
 * `value`/`onValueChange` are the selected chip tokens, `inputValue`/
 * `onInputValueChange` the free-text input, and `onItemHighlighted` reports the
 * cmdk-highlighted row so callers can implement Tab completion.
 */

const ComboboxAnchorContext =
	createContext<RefObject<HTMLDivElement | null> | null>(null);

type ComboboxStateValue = {
	value: string[];
	inputValue: string;
	open: boolean;
	multiple: boolean;
	onInputValueChange?: (value: string, details: { reason: string }) => void;
	select: (value: string) => void;
};

const ComboboxStateContext = createContext<ComboboxStateValue | null>(null);

function useComboboxState(): ComboboxStateValue {
	const context = useContext(ComboboxStateContext);
	if (!context) {
		throw new Error("Combobox components must be used within a <Combobox />");
	}
	return context;
}

type ComboboxProps = {
	multiple?: boolean;
	/** Accepted for API parity; cmdk auto-highlights the first row. */
	autoHighlight?: boolean;
	/** Accepted for API parity; filtering is owned by the caller. */
	filter?: unknown;
	/** Accepted for API parity; opening is caller-controlled. */
	openOnInputClick?: boolean;
	/** Accepted for API parity; navigation order comes from the DOM. */
	items?: readonly string[];
	open?: boolean;
	onOpenChange?: (open: boolean, details: { reason: string }) => void;
	value?: string[];
	onValueChange?: (value: string[]) => void;
	inputValue?: string;
	onInputValueChange?: (value: string, details: { reason: string }) => void;
	onItemHighlighted?: (value: string | undefined) => void;
	/** Accessible label for the input. cmdk wires it via `aria-labelledby`. */
	label?: string;
	children?: ReactNode;
};

export function Combobox({
	multiple = true,
	open = false,
	onOpenChange,
	value = [],
	onValueChange,
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
	const valueRef = useRef(value);
	valueRef.current = value;
	const onValueChangeRef = useRef(onValueChange);
	onValueChangeRef.current = onValueChange;
	const onOpenChangeRef = useRef(onOpenChange);
	onOpenChangeRef.current = onOpenChange;
	const onItemHighlightedRef = useRef(onItemHighlighted);
	onItemHighlightedRef.current = onItemHighlighted;

	const select = (next: string) => {
		const current = valueRef.current;
		const exists = current.includes(next);
		const updated = exists
			? current.filter((entry) => entry !== next)
			: multiple
				? [...current, next]
				: [next];
		onValueChangeRef.current?.(updated);
	};

	// Radix only reports a boolean, so synthesize a reason for the caller. Opens
	// are driven directly by the caller through `open`; Radix mostly reports
	// dismissals (outside press / escape).
	const handleOpenChange = (nextOpen: boolean) => {
		onOpenChangeRef.current?.(nextOpen, {
			reason: nextOpen ? "trigger-press" : "outside-press",
		});
	};

	const state: ComboboxStateValue = {
		value,
		inputValue,
		open,
		multiple,
		onInputValueChange,
		select,
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
					<Popover open={open} onOpenChange={handleOpenChange} modal={false}>
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

type ComboboxContentProps = ComponentPropsWithRef<typeof PopoverContent> & {
	/** Accepted for API parity; the Radix anchor provides the position target. */
	anchor?: unknown;
};

export const ComboboxContent: FC<ComboboxContentProps> = ({
	className,
	align = "start",
	sideOffset = 6,
	anchor: _anchor,
	...props
}) => {
	const anchorRef = useContext(ComboboxAnchorContext);
	// cmdk keeps a running count of the rows it renders; drive the `data-empty`
	// styling group from it so `ComboboxEmpty` can show/hide via CSS.
	const count = useCommandState((state) => state.filtered.count) ?? 0;

	return (
		<PopoverContent
			disablePortal
			align={align}
			sideOffset={sideOffset}
			data-empty={count === 0 ? "" : undefined}
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
	const count = useCommandState((state) => state.filtered.count) ?? 0;

	return (
		<CommandPrimitive.List
			data-slot="combobox-list"
			data-empty={count === 0 ? "" : undefined}
			className={cn(
				"max-h-96 scroll-py-1 overflow-y-auto overscroll-contain p-1",
				className,
			)}
			{...props}
		/>
	);
};

type ComboboxItemProps = ComponentPropsWithRef<typeof CommandPrimitive.Item> & {
	showIndicator?: boolean;
};

export const ComboboxItem: FC<ComboboxItemProps> = ({
	className,
	children,
	showIndicator = true,
	value,
	onSelect,
	...props
}) => {
	const { value: selected, select } = useComboboxState();
	const isSelected = value !== undefined && selected.includes(String(value));

	return (
		<CommandPrimitive.Item
			data-slot="combobox-item"
			value={value}
			onSelect={(selectedValue) => {
				if (value !== undefined) {
					select(String(value));
				}
				onSelect?.(selectedValue);
			}}
			className={cn(
				`relative flex w-full cursor-default select-none items-center gap-2 rounded-sm
				py-1.5 pr-8 pl-2 text-sm text-content-secondary outline-none
				data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary
				data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50
				[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-icon-sm`,
				!showIndicator && "pr-2",
				className,
			)}
			{...props}
		>
			{children}
			{showIndicator && isSelected && (
				<span className="pointer-events-none absolute right-2 flex size-icon-sm items-center justify-center">
					<CheckIcon className="pointer-events-none size-icon-sm" />
				</span>
			)}
		</CommandPrimitive.Item>
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
				className={cn("h-auto min-h-10 w-full", className)}
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
				"flex min-h-8 min-w-0 flex-1 flex-wrap items-center gap-1 self-stretch py-1.5",
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
	const { select } = useComboboxState();
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
							select(chipText);
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
			onValueChange={(next) =>
				onInputValueChange?.(next, { reason: "input-change" })
			}
			className={cn(
				`h-6 min-w-24 flex-1 border-0 bg-transparent p-0 text-sm font-medium
				text-content-primary outline-none placeholder:text-content-secondary`,
				className,
			)}
			{...props}
		/>
	);
};
