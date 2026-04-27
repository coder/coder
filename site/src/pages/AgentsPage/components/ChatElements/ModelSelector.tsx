import type { FC } from "react";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
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

const formatContextLimit = (tokens: number): string => {
	if (tokens >= 1_000_000) {
		const m = tokens / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1)}M context window`;
	}
	const k = Math.round(tokens / 1_000);
	return `${k}K context window`;
};

const getOptionLabel = (option: ModelSelectorOption): string => {
	const displayName = option.displayName.trim();
	if (displayName) {
		return displayName;
	}
	const model = option.model.trim();
	if (model) {
		return model;
	}
	return option.id;
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
	open,
	onOpenChange,
	onTriggerTouchStart,
	enableMobileFullWidthDropdown = false,
}) => {
	const selectedModel = options.find((option) => option.id === value);
	const optionsByProvider = (() => {
		const grouped = new Map<string, ModelSelectorOption[]>();

		for (const option of options) {
			const providerOptions = grouped.get(option.provider);
			if (providerOptions) {
				providerOptions.push(option);
				continue;
			}
			grouped.set(option.provider, [option]);
		}

		return Array.from(grouped.entries());
	})();
	const isDisabled = disabled || options.length === 0;

	return (
		<Select
			value={value}
			onValueChange={onValueChange}
			disabled={isDisabled}
			open={open}
			onOpenChange={onOpenChange}
		>
			<SelectTrigger
				aria-label={selectedModel ? getOptionLabel(selectedModel) : placeholder}
				className={cn(
					"h-8 min-w-0 shrink md:shrink-0 md:w-auto gap-0.5 md:gap-1.5 border-0 bg-transparent px-1 text-xs shadow-none transition-colors hover:bg-transparent hover:text-content-primary focus:ring-0 [&>span]:truncate [&>svg]:shrink-0 [&>svg]:transition-colors [&>svg]:hover:text-content-primary",
					className,
				)}
				onTouchStart={onTriggerTouchStart}
			>
				<SelectValue placeholder={placeholder}>
					{selectedModel ? getOptionLabel(selectedModel) : placeholder}
				</SelectValue>
			</SelectTrigger>
			<SelectContent
				side={dropdownSide}
				align={dropdownAlign}
				className={cn(
					enableMobileFullWidthDropdown &&
						"mobile-full-width-dropdown mobile-full-width-dropdown-bottom",
					"border-border-default [&_[role=option]]:text-xs",
					contentClassName,
				)}
			>
				<TooltipProvider delayDuration={300}>
					{optionsByProvider.map(([provider, providerOptions]) => {
						const providerLabel = formatProviderLabel(provider);
						return (
							<SelectGroup key={provider}>
								{providerOptions.map((option) => (
									<ModelOptionItem
										key={option.id}
										option={option}
										providerLabel={providerLabel}
										isSelected={option.id === value}
									/>
								))}
							</SelectGroup>
						);
					})}
					{options.length === 0 && (
						<SelectItem value="__empty__" disabled>
							{emptyMessage}
						</SelectItem>
					)}
				</TooltipProvider>
			</SelectContent>
		</Select>
	);
};

interface ModelOptionItemProps {
	option: ModelSelectorOption;
	providerLabel: string;
	isSelected: boolean;
}

const ModelOptionItem: FC<ModelOptionItemProps> = ({
	option,
	providerLabel,
	isSelected,
}) => {
	const label = getOptionLabel(option);
	const contextInfo =
		option.contextLimit != null && option.contextLimit > 0
			? formatContextLimit(option.contextLimit)
			: null;
	const subtext = contextInfo
		? `via ${providerLabel}, ${contextInfo}`
		: `via ${providerLabel}`;

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<SelectItem
					value={option.id}
					className={cn(isSelected && "bg-surface-secondary")}
				>
					<span className="flex flex-col">
						<span>{label}</span>
						<span className="text-content-secondary text-[11px] leading-tight md:hidden">
							{subtext}
						</span>
					</span>
				</SelectItem>
			</TooltipTrigger>
			<TooltipContent
				side="right"
				sideOffset={4}
				className="hidden px-2.5 py-1.5 md:block"
			>
				<span className="block font-semibold text-content-primary leading-tight">
					{label} via {providerLabel}
				</span>
				{contextInfo && (
					<span className="block text-content-secondary leading-tight">
						{contextInfo}
					</span>
				)}
			</TooltipContent>
		</Tooltip>
	);
};
