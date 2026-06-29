import { CheckIcon, ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { useMemo, useState } from "react";
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

const formatContextLimitShort = (tokens: number): string => {
	if (tokens >= 1_000_000) {
		const m = tokens / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
	}
	return `${Math.round(tokens / 1_000)}K`;
};

// cmdk filters rows by matching the query against this string and
// resolves the active row by exact match, so the same helper is used
// for both rendering rows and landing on the selection when it opens.
const getCmdkValue = (
	option: ModelSelectorOption,
	providerLabel: string,
): string =>
	[
		option.displayName,
		option.provider,
		option.model,
		providerLabel,
		option.contextLimit != null && option.contextLimit > 0
			? formatContextLimitShort(option.contextLimit)
			: "",
	]
		.join(" ")
		.toLowerCase();

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
	const isOpen = controlledOpen ?? uncontrolledOpen;
	const setOpen = (next: boolean) => {
		if (controlledOpen === undefined) {
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
	const selectedCmdkValue = selectedModel
		? getCmdkValue(selectedModel, formatProviderLabel(selectedModel.provider))
		: undefined;

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
						"inline-flex h-7 min-w-0 shrink items-center gap-1 rounded-md px-2 text-xs font-medium md:shrink-0 md:w-auto",
						"border-0 bg-transparent text-content-primary",
						"transition-colors hover:bg-surface-secondary",
						"data-[state=open]:bg-surface-secondary",
						"focus:outline-none focus:ring-0",
						"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
						"disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent",
						"[&>span]:truncate",
						className,
					)}
				>
					<span className="truncate">{triggerLabel}</span>
					<ChevronDownIcon
						aria-hidden="true"
						className="size-3.5 shrink-0 text-content-secondary"
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				sideOffset={6}
				className={cn(
					"w-72 p-0",
					"border border-solid border-border-default",
					"bg-surface-primary",
					enableMobileFullWidthDropdown &&
						"mobile-full-width-dropdown mobile-full-width-dropdown-bottom",
					contentClassName,
				)}
			>
				<TooltipProvider delayDuration={300}>
					<Command
						defaultValue={selectedCmdkValue}
						// Override cmdk's fuzzy filter with strict substring matching
						// so typing a model name hides unrelated providers cleanly.
						filter={(val, search) => {
							const needle = search.trim().toLowerCase();
							if (!needle) {
								return 1;
							}
							return val.toLowerCase().includes(needle) ? 1 : 0;
						}}
						className="border-0 bg-transparent"
					>
						<CommandInput
							placeholder="Search..."
							className="h-10 text-sm"
							aria-label="Search models"
						/>
						<CommandList className="max-h-[280px]">
							<CommandEmpty className="py-6 text-center text-sm text-content-secondary">
								{options.length === 0 ? emptyMessage : "No matching models."}
							</CommandEmpty>
							{optionsByProvider.map(([provider, providerOptions]) => {
								const providerLabel = formatProviderLabel(provider);
								return (
									<CommandGroup
										key={provider}
										heading={providerLabel}
										className="px-1.5 py-1.5 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-2 [&_[cmdk-group-heading]]:text-[13px] [&_[cmdk-group-heading]]:font-normal [&_[cmdk-group-heading]]:text-content-secondary [&_[cmdk-group-items]]:flex [&_[cmdk-group-items]]:flex-col [&_[cmdk-group-items]]:gap-1"
									>
										{providerOptions.map((option) => (
											<ModelRow
												key={option.id}
												option={option}
												providerLabel={providerLabel}
												isSelected={option.id === value}
												onSelect={() => {
													onValueChange(option.id);
													setOpen(false);
												}}
											/>
										))}
									</CommandGroup>
								);
							})}
						</CommandList>
					</Command>
				</TooltipProvider>
			</PopoverContent>
		</Popover>
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

	const row = (
		<CommandItem
			value={getCmdkValue(option, providerLabel)}
			onSelect={onSelect}
			aria-selected={isSelected}
			// Keep the accessible name as just the display name so the
			// context chip does not leak into screen-reader labels.
			aria-label={option.displayName}
			className={cn(
				"group flex h-10 cursor-pointer items-center gap-2 rounded-md px-2.5 py-0",
				"text-sm font-normal text-content-primary",
				// cmdk's data-selected is the keyboard/hover cursor, not the
				// persistent selection. Use a faint overlay for the cursor so
				// it does not collide visually with the selected row below.
				"data-[selected=true]:bg-content-primary/[0.08]",
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
				{contextShort && (
					<span className="block text-content-secondary leading-tight">
						{contextShort} context window
					</span>
				)}
			</TooltipContent>
		</Tooltip>
	);
};
