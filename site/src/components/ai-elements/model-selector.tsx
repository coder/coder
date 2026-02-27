import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { type FC, useMemo } from "react";
import { cn } from "utils/cn";

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
}) => {
	const selectedModel = useMemo(
		() => options.find((option) => option.id === value),
		[options, value],
	);
	const optionsByProvider = useMemo(() => {
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
	}, [options]);
	const showProviderHeading = optionsByProvider.length > 1;
	const isDisabled = disabled || options.length === 0;

	return (
		<Select value={value} onValueChange={onValueChange} disabled={isDisabled}>
			<SelectTrigger
				className={cn(
					"h-8 w-auto gap-1.5 border-none bg-transparent px-1 text-xs shadow-none transition-colors hover:bg-transparent hover:text-content-primary [&>svg]:transition-colors [&>svg]:hover:text-content-primary focus:ring-0 focus-visible:ring-0",
					className,
				)}
			>
				<SelectValue placeholder={placeholder}>
					{selectedModel?.displayName ?? placeholder}
				</SelectValue>
			</SelectTrigger>
			<SelectContent
				side={dropdownSide}
				align={dropdownAlign}
				className={cn("[&_[role=option]]:text-xs", contentClassName)}
			>
				<TooltipProvider delayDuration={300}>
					{optionsByProvider.map(([provider, providerOptions]) => {
						const providerLabel = formatProviderLabel(provider);
						return (
							<SelectGroup key={provider}>
								{showProviderHeading && (
									<SelectLabel>{providerLabel}</SelectLabel>
								)}
								{providerOptions.map((option) => (
									<ModelOptionItem
										key={option.id}
										option={option}
										providerLabel={providerLabel}
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
}

const ModelOptionItem: FC<ModelOptionItemProps> = ({
	option,
	providerLabel,
}) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<SelectItem value={option.id}>{option.displayName}</SelectItem>
			</TooltipTrigger>
			<TooltipContent side="right" sideOffset={4} className="px-2.5 py-1.5">
				<span className="block font-semibold text-content-primary leading-tight">
					{option.displayName} via {providerLabel}
				</span>
				{option.contextLimit != null && option.contextLimit > 0 && (
					<span className="block text-content-secondary leading-tight">
						{formatContextLimit(option.contextLimit)}
					</span>
				)}
			</TooltipContent>
		</Tooltip>
	);
};
