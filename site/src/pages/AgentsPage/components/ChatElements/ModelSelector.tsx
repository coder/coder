import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
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
import { cn } from "#/utils/cn";

export interface ModelSelectorOption {
	id: string;
	provider: string;
	model: string;
	displayName: string;
	contextLimit?: number;
	/** The specific provider config instance ID, used to group models
	 * when multiple providers of the same type exist. */
	providerConfigId?: string;
	/** Human-readable label for the provider instance (e.g. "Anthropic Work"). */
	providerDisplayName?: string;
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

const formatContextLimit = (tokens: number): string => {
	if (tokens >= 1_000_000) {
		const m = tokens / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
	}
	const k = Math.round(tokens / 1_000);
	return `${k}K`;
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
	onOpenChange: controlledOnOpenChange,
	onTriggerTouchStart,
	enableMobileFullWidthDropdown = false,
}) => {
	const [internalOpen, setInternalOpen] = useState(false);
	const isOpen = controlledOpen ?? internalOpen;
	const setIsOpen = controlledOnOpenChange ?? setInternalOpen;

	const selectedModel = options.find((option) => option.id === value);
	const optionsByProvider = useMemo(() => {
		const grouped = new Map<string, ModelSelectorOption[]>();

		for (const option of options) {
			// Group by provider config instance when available,
			// falling back to provider type for models without
			// a specific provider config.
			const groupKey = option.providerConfigId || option.provider;
			const providerOptions = grouped.get(groupKey);
			if (providerOptions) {
				providerOptions.push(option);
				continue;
			}
			grouped.set(groupKey, [option]);
		}

		return Array.from(grouped.entries());
	}, [options]);

	const isDisabled = disabled || options.length === 0;

	return (
		<Popover open={isOpen} onOpenChange={setIsOpen}>
			<PopoverTrigger asChild disabled={isDisabled}>
				<button
					type="button"
					aria-label={selectedModel ? selectedModel.displayName : placeholder}
					className={cn(
						"inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full border-0 bg-surface-secondary px-2 py-0.5 text-xs font-medium text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary",
						isDisabled && "pointer-events-none opacity-50",
						className,
					)}
					onTouchStart={onTriggerTouchStart}
				>
					<span className="truncate max-w-32">
						{selectedModel ? selectedModel.displayName : placeholder}
					</span>
						<ChevronDownIcon
							strokeWidth={2.5}
							className={cn(
								"size-3.5 shrink-0 transition-transform",
								isOpen && "rotate-180",
							)}
						/>

				</button>
			</PopoverTrigger>
			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				className={cn(
					"w-auto min-w-50 max-w-80 p-0 overflow-hidden",
					enableMobileFullWidthDropdown &&
						"mobile-full-width-dropdown mobile-full-width-dropdown-bottom",
					contentClassName,
				)}
			>
				<Command
					className="bg-surface-primary"
					filter={(value, search, keywords) => {
						const searchLower = search.toLowerCase();
						const extendedValue = (keywords?.join(" ") ?? value).toLowerCase();
						if (extendedValue.includes(searchLower)) {
							return 1;
						}
						return 0;
					}}
				>
					{options.length >= 7 && (
						<CommandInput placeholder="Search..." className="text-xs" />
					)}
					<CommandList className="max-h-72 [scrollbar-width:thin]">
						<CommandEmpty className="text-xs">{emptyMessage}</CommandEmpty>
						{optionsByProvider.map(
							([_groupKey, providerOptions]) => {
								const first = providerOptions[0];
								const provider = first.provider;
								const providerLabel =
									first.providerDisplayName ??
									formatProviderLabel(provider);
								const groupKey =
									first.providerConfigId || provider;
								return (
									<CommandGroup
										key={groupKey}
										className="[&_[cmdk-group-heading]]:text-content-secondary [&_[cmdk-group-heading]]:select-none [&_[cmdk-group-heading]]:pointer-events-none [&:not(:first-child)]:border-0 [&:not(:first-child)]:border-t [&:not(:first-child)]:border-solid [&:not(:first-child)]:border-surface-secondary [&_[cmdk-group-items]]:flex [&_[cmdk-group-items]]:flex-col [&_[cmdk-group-items]]:gap-0.5"
									heading={
										<span className="text-xs text-content-secondary">{providerLabel}</span>
									}
								>
									{providerOptions.map((option) => {
										const isSelected = option.id === value;
										const contextInfo =
											option.contextLimit != null && option.contextLimit > 0
												? formatContextLimit(option.contextLimit)
												: null;
										return (
											<CommandItem
												key={option.id}
												value={option.id}
												keywords={[
													option.displayName,
													option.model,
													provider,
													providerLabel,
												]}
												onSelect={() => {
													onValueChange(option.id);
													setIsOpen(false);
												}}
												className={cn("cursor-pointer py-1.5 text-xs", isSelected && "bg-surface-secondary text-content-primary")}
											>
												<span className="flex flex-1 items-center justify-between gap-2">
													<span className="truncate">
														{option.displayName}
														{contextInfo && (
															<span className="text-xs text-content-secondary">
																{" "}({contextInfo})
															</span>
														)}
													</span>
													{isSelected && (
														<CheckIcon className="size-4 shrink-0 text-content-primary" />
													)}
												</span>
											</CommandItem>
										);
									})}
								</CommandGroup>
							);
						},
					)}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
