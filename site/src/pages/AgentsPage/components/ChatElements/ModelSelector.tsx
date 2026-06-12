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

	return (
		<TooltipProvider delayDuration={300}>
			<Command
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
				<CommandList className="max-h-[280px] border-t-0">
					<CommandEmpty className="py-6 text-center text-sm text-content-secondary">
						{options.length === 0 ? emptyMessage : "No matching models."}
					</CommandEmpty>
					{optionsByProvider.map(([provider, providerOptions]) => {
						const providerLabel = formatProviderLabel(provider);
						return (
							<CommandGroup
								key={provider}
								heading={providerLabel}
								// Spacing tightened to match the dense vertical
								// rhythm in the design.
								className="px-1.5 py-1.5 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-[13px] [&_[cmdk-group-heading]]:font-normal [&_[cmdk-group-heading]]:text-content-secondary"
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
				{effort && (
					<EffortRow
						effort={effort}
						providerLabel={
							selectedModel
								? formatProviderLabel(selectedModel.provider)
								: undefined
						}
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
	const cmdkValue = [
		option.displayName,
		option.provider,
		option.model,
		providerLabel,
		contextShort ?? "",
	]
		.join(" ")
		.toLowerCase();

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
				"group flex h-9 cursor-pointer items-center gap-2 rounded-md px-2 py-0",
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
 * Read-only display of the model's admin-configured reasoning effort.
 * The slider is non-interactive on purpose: per-chat effort selection
 * requires a backend change (see PR description).
 */
const EffortRow: FC<EffortRowProps> = ({ effort, providerLabel }) => {
	const tooltipId = useId();
	const [hint, setHint] = useState<string>("");
	useEffect(() => {
		setHint(
			providerLabel
				? `Configured on the ${providerLabel} model. Per-chat override coming soon.`
				: "Configured on the model. Per-chat override coming soon.",
		);
	}, [providerLabel]);

	const sliderValue = EFFORT_LEVELS.indexOf(effort);
	const label = EFFORT_LABELS[effort];

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
						className="max-w-[220px] px-2.5 py-1.5"
					>
						<span className="block text-content-primary leading-snug">
							{hint}
						</span>
					</TooltipContent>
				</Tooltip>
			</div>
			<Slider
				aria-label="Reasoning effort"
				aria-readonly="true"
				aria-valuetext={label}
				min={0}
				max={EFFORT_LEVELS.length - 1}
				step={1}
				value={[sliderValue]}
				disabled
				// `Slider` dims the track at 40% when disabled; the read-only
				// effort display should stay at full opacity since it is
				// communicating real configuration.
				className="grow opacity-100 data-[disabled]:opacity-100 [&_[data-disabled]]:opacity-100"
			/>
			<span
				className={cn(
					"inline-flex h-6 shrink-0 items-center rounded-md px-2",
					"border border-solid border-border-default bg-surface-secondary",
					"text-xs font-medium text-content-primary",
				)}
			>
				{label}
			</span>
		</div>
	);
};
