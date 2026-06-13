import { CheckIcon, ChevronDownIcon, InfoIcon } from "lucide-react";
import type { FC } from "react";
import { useEffect, useId, useMemo, useState } from "react";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Slider } from "#/components/Slider/Slider";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

export interface ModelSelectorOption {
	id: string;
	provider: string;
	model: string;
	displayName: string;
	contextLimit?: number;
	/**
	 * Reasoning effort configured on the model (admin-set). Used to
	 * render the read-only Effort row in the model picker. One of
	 * "none", "minimal", "low", "medium", "high", "xhigh".
	 */
	effort?: string;
}

interface ModelSelectorProps {
	options: readonly ModelSelectorOption[];
	value: string;
	onValueChange: (value: string) => void;
	disabled?: boolean;
	placeholder?: string;
	emptyMessage?: string;
	formatProviderLabel?: (provider: string) => string;
	className?: string;
	dropdownSide?: "top" | "bottom" | "left" | "right";
	dropdownAlign?: "start" | "center" | "end";
	contentClassName?: string;
	open?: boolean;
	onOpenChange?: (open: boolean) => void;
	onTriggerTouchStart?: () => void;
	enableMobileFullWidthDropdown?: boolean;
}

const defaultFormatProviderLabel = (provider: string): string => {
	const normalized = provider.trim().toLowerCase();
	if (!normalized) {
		return "Unknown";
	}
	return `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
};

/**
 * Compact context window label for the inline subtitle next to a
 * model name, e.g. "200K" or "1M".
 */
const formatContextLimitShort = (tokens: number): string => {
	if (tokens >= 1_000_000) {
		const m = tokens / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
	}
	const k = Math.round(tokens / 1_000);
	return `${k}K`;
};

const formatContextLimitVerbose = (tokens: number): string =>
	`${formatContextLimitShort(tokens)} context window`;

/**
 * Ordered list of supported reasoning-effort levels, low to high.
 * Mirrors the enum on the backend ChatModel provider options.
 */
const EFFORT_LEVELS = [
	"none",
	"minimal",
	"low",
	"medium",
	"high",
	"xhigh",
] as const;

type EffortLevel = (typeof EFFORT_LEVELS)[number];

const EFFORT_LABELS: Record<EffortLevel, string> = {
	none: "None",
	minimal: "Minimal",
	low: "Low",
	medium: "Medium",
	high: "High",
	xhigh: "Xhigh",
};

const normalizeEffort = (raw: string | undefined): EffortLevel | null => {
	if (!raw) {
		return null;
	}
	const normalized = raw.trim().toLowerCase();
	return (EFFORT_LEVELS as readonly string[]).includes(normalized)
		? (normalized as EffortLevel)
		: null;
};

/**
 * Builds the searchable string that we hand to cmdk as a row's
 * `value`. cmdk filters and highlights items by matching the search
 * query against this string, and looks up the currently active item
 * by exact-match against it, so the same helper is used both when
 * rendering rows and when telling cmdk which row to land on when
 * the dropdown opens.
 */
const getCmdkValue = (
	option: ModelSelectorOption,
	providerLabel: string,
): string => {
	const contextShort =
		option.contextLimit != null && option.contextLimit > 0
			? formatContextLimitShort(option.contextLimit)
			: "";
	return [
		option.displayName,
		option.provider,
		option.model,
		providerLabel,
		contextShort,
	]
		.join(" ")
		.toLowerCase();
};

export const ModelSelector: FC<ModelSelectorProps> = ({
	options,
	value,
	onValueChange,
	disabled = false,
	placeholder = "Select model",
	emptyMessage = "No models found.",
	formatProviderLabel = defaultFormatProviderLabel,
	className,
	dropdownSide = "bottom",
	dropdownAlign = "start",
	contentClassName,
	open: controlledOpen,
	onOpenChange,
	onTriggerTouchStart,
	enableMobileFullWidthDropdown = false,
}) => {
	const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
	const isControlled = controlledOpen !== undefined;
	const isOpen = isControlled ? controlledOpen : uncontrolledOpen;

	const setOpen = (next: boolean) => {
		if (!isControlled) {
			setUncontrolledOpen(next);
		}
		onOpenChange?.(next);
	};

	const selectedModel = options.find((option) => option.id === value);
	const optionsByProvider = useMemo(() => {
		const grouped = new Map<string, ModelSelectorOption[]>();
		for (const option of options) {
			const existing = grouped.get(option.provider);
			if (existing) {
				existing.push(option);
				continue;
			}
			grouped.set(option.provider, [option]);
		}
		return Array.from(grouped.entries());
	}, [options]);

	const isDisabled = disabled || options.length === 0;
	const triggerLabel = selectedModel ? selectedModel.displayName : placeholder;

	return (
		<Popover
			open={isOpen}
			onOpenChange={(next) => {
				if (isDisabled && next) {
					return;
				}
				setOpen(next);
			}}
		>
			<PopoverTrigger asChild>
				<button
					type="button"
					role="combobox"
					aria-label={triggerLabel}
					aria-expanded={isOpen}
					aria-haspopup="listbox"
					disabled={isDisabled}
					onTouchStart={onTriggerTouchStart}
					className={cn(
						// Inline-friendly chip that fits next to other chat-footer controls.
						"inline-flex h-7 min-w-0 shrink items-center gap-1 rounded-md px-2 text-xs font-medium md:shrink-0 md:w-auto",
						"border-0 bg-transparent text-content-primary",
						"transition-colors hover:bg-surface-secondary",
						// Suppress the mouse-focus ring, keep the keyboard-focus ring.
						"focus:outline-none focus:ring-0",
						"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
						"disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent",
						"data-[state=open]:bg-surface-secondary",
						className,
					)}
				>
					<span className="truncate">{triggerLabel}</span>
					<ChevronDownIcon
						aria-hidden="true"
						className="size-3.5 shrink-0 text-content-secondary transition-colors group-hover:text-content-primary"
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				sideOffset={6}
				className={cn(
					// Width chosen to comfortably fit "Display Name (1M)" rows.
					"w-72 p-0",
					"border border-solid border-border-default",
					"bg-surface-primary",
					enableMobileFullWidthDropdown &&
						"mobile-full-width-dropdown mobile-full-width-dropdown-bottom",
					contentClassName,
				)}
			>
				<ModelPickerPanel
					options={options}
					optionsByProvider={optionsByProvider}
					selectedId={value}
					selectedModel={selectedModel}
					emptyMessage={emptyMessage}
					formatProviderLabel={formatProviderLabel}
					onSelect={(id) => {
						onValueChange(id);
						setOpen(false);
					}}
				/>
			</PopoverContent>
		</Popover>
	);
};

interface ModelPickerPanelProps {
	options: readonly ModelSelectorOption[];
	optionsByProvider: readonly (readonly [string, ModelSelectorOption[]])[];
	selectedId: string;
	selectedModel: ModelSelectorOption | undefined;
	emptyMessage: string;
	formatProviderLabel: (provider: string) => string;
	onSelect: (id: string) => void;
}

const ModelPickerPanel: FC<ModelPickerPanelProps> = ({
	options,
	optionsByProvider,
	selectedId,
	selectedModel,
	emptyMessage,
	formatProviderLabel,
	onSelect,
}) => {
	const effort = normalizeEffort(selectedModel?.effort);
	// When the dropdown opens, land cmdk's active row on the
	// currently-selected model. cmdk scrolls the active row into view
	// on mount, so this guarantees the user sees their choice without
	// having to scroll. Recomputed each render so it stays in sync if
	// the selection changes while the popover is open.
	const selectedCmdkValue = selectedModel
		? getCmdkValue(selectedModel, formatProviderLabel(selectedModel.provider))
		: undefined;

	return (
		<TooltipProvider delayDuration={300}>
			<Command
				defaultValue={selectedCmdkValue}
				// The default cmdk filter scores fuzzy character overlaps and
				// keeps loosely matching rows visible. The model picker needs
				// a strict, substring-based filter so typing a model name
				// hides unrelated providers cleanly.
				filter={(value, search) => {
					const needle = search.trim().toLowerCase();
					if (!needle) {
						return 1;
					}
					return value.toLowerCase().includes(needle) ? 1 : 0;
				}}
				className="border-0 bg-transparent"
			>
				<CommandInput
					placeholder="Search..."
					className="h-10 text-sm"
					aria-label="Search models"
				/>
				<CommandList
					className={cn(
						// Keep the default top border from `Command`'s list (drawn
						// with the theme `border-border` token) so there is a clean
						// rule between the search input and the model rows in both
						// dark and light themes.
						"max-h-[280px]",
						// Slimmer custom scrollbar. The 8px width keeps a usable
						// hit area for the thumb while taking less visual weight
						// than the default browser scrollbar. `scrollbar-width:
						// thin` covers Firefox; the `::-webkit-scrollbar` rules
						// cover Chromium and Safari.
						"[scrollbar-width:thin]",
						"[&::-webkit-scrollbar]:w-2",
						"[&::-webkit-scrollbar-track]:bg-transparent",
						"[&::-webkit-scrollbar-thumb]:rounded-full",
						"[&::-webkit-scrollbar-thumb]:bg-surface-quaternary",
						"[&::-webkit-scrollbar-thumb:hover]:bg-surface-tertiary",
					)}
				>
					<CommandEmpty className="py-6 text-center text-sm text-content-secondary">
						{options.length === 0 ? emptyMessage : "No matching models."}
					</CommandEmpty>
					{optionsByProvider.map(([provider, providerOptions]) => {
						const providerLabel = formatProviderLabel(provider);
						return (
							<CommandGroup
								key={provider}
								heading={providerLabel}
								// Tightened spacing matches the dense vertical rhythm
								// in the design. The 2px gap between items keeps each
								// row's hover background visually separated from its
								// neighbours so mouseover state has a clear edge.
								className="px-1.5 py-1.5 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-2 [&_[cmdk-group-heading]]:text-[13px] [&_[cmdk-group-heading]]:font-normal [&_[cmdk-group-heading]]:text-content-secondary [&_[cmdk-group-items]]:flex [&_[cmdk-group-items]]:flex-col [&_[cmdk-group-items]]:gap-1"
							>
								{providerOptions.map((option) => (
									<ModelRow
										key={option.id}
										option={option}
										providerLabel={providerLabel}
										isSelected={option.id === selectedId}
										onSelect={() => onSelect(option.id)}
									/>
								))}
							</CommandGroup>
						);
					})}
				</CommandList>
				{selectedModel && (
					<EffortRow
						// DEMO: while the backend does not yet plumb a per-chat
						// `reasoning_effort`, every model gets an Effort row. If
						// the admin configured an effort, we use it as the cap;
						// otherwise we fall back to `xhigh` so the slider has
						// the full range to show. Drop the fallback once the
						// backend lands and the row should hide for models that
						// do not support reasoning.
						effort={effort ?? "xhigh"}
						providerLabel={formatProviderLabel(selectedModel.provider)}
					/>
				)}
			</Command>
		</TooltipProvider>
	);
};

interface ModelRowProps {
	option: ModelSelectorOption;
	providerLabel: string;
	isSelected: boolean;
	onSelect: () => void;
}

const ModelRow: FC<ModelRowProps> = ({
	option,
	providerLabel,
	isSelected,
	onSelect,
}) => {
	const contextShort =
		option.contextLimit != null && option.contextLimit > 0
			? formatContextLimitShort(option.contextLimit)
			: null;
	const contextVerbose =
		option.contextLimit != null && option.contextLimit > 0
			? formatContextLimitVerbose(option.contextLimit)
			: null;
	// Pack searchable text into cmdk's `value` so the search input
	// matches name, provider, and the short context label.
	const cmdkValue = getCmdkValue(option, providerLabel);

	const row = (
		<CommandItem
			value={cmdkValue}
			onSelect={onSelect}
			aria-selected={isSelected}
			// Explicit accessible name avoids leaking the visible "(200K)"
			// chip into the screen-reader/test label. The model name alone
			// is the stable identifier for callers like
			// `getByRole("option", { name: ... })`.
			aria-label={option.displayName}
			data-selected-state={isSelected ? "selected" : undefined}
			className={cn(
				// Reset the default cmdk "selected = highlighted" treatment so
				// keyboard focus does not visually collide with the chosen row.
				"group flex h-10 cursor-pointer items-center gap-2 rounded-md px-2.5 py-0",
				"text-sm font-normal text-content-primary",
				"data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary",
				// The persistent "this is the chosen value" treatment.
				isSelected && "bg-surface-secondary text-content-primary",
			)}
		>
			<span className="min-w-0 flex-1 truncate">
				<span>{option.displayName}</span>
				{contextShort && (
					<span className="ml-2 text-content-secondary">({contextShort})</span>
				)}
			</span>
			{isSelected && (
				<CheckIcon
					aria-hidden="true"
					className="size-4 shrink-0 text-content-primary"
				/>
			)}
		</CommandItem>
	);

	return (
		<Tooltip>
			<TooltipTrigger asChild>{row}</TooltipTrigger>
			<TooltipContent
				side="right"
				sideOffset={8}
				className="hidden px-2.5 py-1.5 md:block"
			>
				<span className="block font-semibold text-content-primary leading-tight">
					{option.displayName} via {providerLabel}
				</span>
				{contextVerbose && (
					<span className="block text-content-secondary leading-tight">
						{contextVerbose}
					</span>
				)}
			</TooltipContent>
		</Tooltip>
	);
};

interface EffortRowProps {
	effort: EffortLevel;
	providerLabel?: string;
}

/**
 * Interactive effort picker, capped at the admin-configured value.
 *
 * NOTE: this is a frontend-only demo. The selected value lives in
 * local state and is never sent to the backend; the chat behaves
 * the same as if the slider were not touched. Making the user's
 * choice take effect requires a backend change to plumb a per-chat
 * `reasoning_effort` through to the provider call.
 */
const EffortRow: FC<EffortRowProps> = ({ effort, providerLabel }) => {
	const tooltipId = useId();
	const adminIndex = EFFORT_LEVELS.indexOf(effort);
	// Local-only state: starts at the admin's value, can be dragged
	// down. Resets each time the dropdown is reopened (the EffortRow
	// remounts).
	const [selectedIndex, setSelectedIndex] = useState(adminIndex);
	// If the admin's value changes (e.g. user picks a different model),
	// snap back to the new admin max so the slider stays within bounds.
	useEffect(() => {
		setSelectedIndex(adminIndex);
	}, [adminIndex]);

	const clamped = Math.min(Math.max(selectedIndex, 0), adminIndex);
	const currentLevel = EFFORT_LEVELS[clamped] ?? effort;
	const currentLabel = EFFORT_LABELS[currentLevel];
	const hint = providerLabel
		? `Drag to lower the effort below the ${providerLabel} model's configured max (${EFFORT_LABELS[effort]}). Preview only.`
		: `Drag to lower the effort below the model's configured max (${EFFORT_LABELS[effort]}). Preview only.`;

	return (
		<div
			className={cn(
				// Top divider, generous padding so the row breathes.
				"flex items-center gap-3 border-0 border-t border-solid border-border-default",
				"px-3 py-3",
			)}
		>
			<div className="flex shrink-0 items-center gap-1 text-xs font-medium text-content-secondary">
				<span>Effort</span>
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							aria-describedby={tooltipId}
							aria-label="Effort information"
							className={cn(
								"inline-flex size-4 items-center justify-center rounded-full",
								"border-0 bg-transparent p-0 text-content-secondary",
								"transition-colors hover:text-content-primary",
								"focus:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
							)}
						>
							<InfoIcon aria-hidden="true" className="size-3.5" />
						</button>
					</TooltipTrigger>
					<TooltipContent
						id={tooltipId}
						side="top"
						className="max-w-[240px] px-2.5 py-1.5"
					>
						<span className="block text-content-primary leading-snug">
							{hint}
						</span>
					</TooltipContent>
				</Tooltip>
			</div>
			<Slider
				aria-label="Reasoning effort"
				aria-valuetext={currentLabel}
				min={0}
				// Cap the slider's max at the admin-configured effort so the
				// user can only lower it, not raise it past the model's limit.
				max={Math.max(adminIndex, 1)}
				step={1}
				value={[clamped]}
				onValueChange={(values) => {
					const next = values[0];
					if (typeof next === "number") {
						setSelectedIndex(next);
					}
				}}
				className={cn(
					"grow",
					// Smooth value changes so dragging and arrow keys glide
					// between effort levels instead of snapping abruptly. The
					// duration is short enough that pointer drags still feel
					// responsive at each step boundary.
					"[&_[data-orientation=horizontal]]:transition-[transform,left,width] [&_[data-orientation=horizontal]]:duration-150 [&_[data-orientation=horizontal]]:ease-out",
					// Small solid-white thumb. The default Radix thumb is
					// 16px with a border; override to a 10px solid-white
					// circle with a grab/grabbing cursor so it reads as
					// draggable without dominating the row.
					"[&_[role=slider]]:h-2.5 [&_[role=slider]]:w-2.5",
					"[&_[role=slider]]:border-0 [&_[role=slider]]:bg-content-primary",
					"[&_[role=slider]]:shadow-none",
					"[&_[role=slider]]:cursor-grab",
					"[&_[role=slider]:active]:cursor-grabbing",
					"[&_[role=slider]]:hover:border-0",
					"[&_[role=slider]]:focus-visible:ring-0",
				)}
			/>
			<span
				className={cn(
					// Fixed width plus `justify-start` keeps the badge's
					// right edge anchored as the label text grows or shrinks,
					// so the slider track length stays constant.
					"inline-flex h-6 w-[4.5rem] shrink-0 items-center justify-start rounded-md px-2",
					"border border-solid border-border-default bg-surface-secondary",
					"text-xs font-medium text-content-primary tabular-nums",
				)}
			>
				{currentLabel}
			</span>
		</div>
	);
};
