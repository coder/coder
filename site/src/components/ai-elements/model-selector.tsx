import { Button } from "components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import {
	Popover,
	PopoverContent,
	type PopoverContentProps,
	PopoverTrigger,
} from "components/Popover/Popover";
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { cn } from "utils/cn";

export interface ModelSelectorOption {
	id: string;
	provider: string;
	model: string;
	displayName: string;
}

interface ModelSelectorProps {
	options: readonly ModelSelectorOption[];
	value: string;
	onValueChange: (value: string) => void;
	disabled?: boolean;
	placeholder?: string;
	searchPlaceholder?: string;
	emptyMessage?: string;
	formatProviderLabel?: (provider: string) => string;
	className?: string;
	dropdownSide?: PopoverContentProps["side"];
	dropdownAlign?: PopoverContentProps["align"];
	contentClassName?: string;
}

const defaultFormatProviderLabel = (provider: string): string => {
	const normalized = provider.trim().toLowerCase();
	if (!normalized) {
		return "Unknown";
	}
	return `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
};

export const ModelSelector: FC<ModelSelectorProps> = ({
	options,
	value,
	onValueChange,
	disabled = false,
	placeholder = "Select model",
	searchPlaceholder = "Search models...",
	emptyMessage = "No models found.",
	formatProviderLabel = defaultFormatProviderLabel,
	className,
	dropdownSide = "bottom",
	dropdownAlign = "start",
	contentClassName,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");

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

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		if (!nextOpen) {
			setSearch("");
		}
	};

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="outline"
					size="sm"
					disabled={isDisabled}
					className={cn(
						"h-9 w-full justify-between gap-2 text-xs font-normal",
						className,
					)}
				>
					<span
						className={cn(
							"min-w-0 truncate",
							selectedModel ? "text-content-primary" : "text-content-secondary",
						)}
					>
						{selectedModel?.displayName ?? placeholder}
					</span>
					<ChevronsUpDownIcon className="h-4 w-4 shrink-0 text-content-secondary" />
				</Button>
			</PopoverTrigger>

			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				className={cn("w-[min(30rem,calc(100vw-2rem))] p-0", contentClassName)}
			>
				<Command loop>
					<CommandInput
						autoFocus
						placeholder={searchPlaceholder}
						value={search}
						onValueChange={setSearch}
					/>
					<CommandList className="max-h-[min(56vh,24rem)]">
						<CommandEmpty>{emptyMessage}</CommandEmpty>
						{optionsByProvider.map(([provider, providerOptions]) => {
							const providerLabel = formatProviderLabel(provider);
							return (
								<CommandGroup
									key={provider}
									heading={showProviderHeading ? providerLabel : undefined}
								>
									{providerOptions.map((option) => (
										<CommandItem
											key={option.id}
											value={`${option.displayName} ${option.model} ${providerLabel}`}
											onSelect={() => {
												onValueChange(option.id);
												handleOpenChange(false);
											}}
										>
											<div className="flex min-w-0 flex-1 flex-col gap-0.5">
												<span className="truncate text-sm text-content-primary">
													{option.displayName}
												</span>
												<span className="truncate text-2xs text-content-secondary">
													{option.model}
												</span>
											</div>
											<CheckIcon
												className={cn(
													"h-4 w-4",
													option.id === selectedModel?.id
														? "opacity-100"
														: "opacity-0",
												)}
											/>
										</CommandItem>
									))}
								</CommandGroup>
							);
						})}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
