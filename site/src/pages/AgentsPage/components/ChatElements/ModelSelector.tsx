import { CheckIcon } from "lucide-react";
import { type FC, useState } from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
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
import { formatProviderLabel as defaultFormatProviderLabel } from "#/utils/aiProviders";
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
	onTriggerTouchStart?: () => void;
	enableMobileFullWidthDropdown?: boolean;
}

const formatContextLimit = (tokens: number): string => {
	if (tokens >= 1_000_000) {
		const m = tokens / 1_000_000;
		return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
	}
	const k = Math.round(tokens / 1_000);
	return `${k}K`;
};

const getSearchText = (option: ModelSelectorOption, providerLabel: string) =>
	[
		providerLabel,
		option.provider,
		option.displayName,
		option.model,
		option.contextLimit ? formatContextLimit(option.contextLimit) : "",
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
	onTriggerTouchStart,
	enableMobileFullWidthDropdown = false,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen) {
			setSearch("");
		}
		setOpen(nextOpen);
	};
	const selectedModel = options.find((option) => option.id === value);
	const isDisabled = disabled || options.length === 0;
	const query = search.trim().toLowerCase();
	const optionsByProvider = (() => {
		const grouped = new Map<string, ModelSelectorOption[]>();

		for (const option of options) {
			const providerLabel = formatProviderLabel(option.provider);
			if (query && !getSearchText(option, providerLabel).includes(query)) {
				continue;
			}

			const providerOptions = grouped.get(option.provider);
			if (providerOptions) {
				providerOptions.push(option);
				continue;
			}
			grouped.set(option.provider, [option]);
		}

		return Array.from(grouped.entries());
	})();

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild disabled={isDisabled}>
				<Button
					aria-label={selectedModel ? selectedModel.displayName : placeholder}
					aria-expanded={open}
					aria-haspopup="listbox"
					disabled={isDisabled}
					role="combobox"
					type="button"
					variant="subtle"
					className={cn(
						"h-8 min-w-0 shrink justify-start gap-0.5 border-0 bg-transparent px-1 text-xs font-medium shadow-none transition-colors hover:bg-transparent hover:text-content-primary focus:ring-0 focus-visible:ring-2 focus-visible:ring-content-link md:w-auto md:shrink-0 md:gap-1.5 [&>svg]:shrink-0 [&>svg]:transition-colors [&>svg]:hover:text-content-primary",
						className,
					)}
					onTouchStart={onTriggerTouchStart}
				>
					<span className="truncate">
						{selectedModel ? selectedModel.displayName : placeholder}
					</span>
					<ChevronDownIcon open={open} className="size-icon-sm" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				className={cn(
					enableMobileFullWidthDropdown &&
						"mobile-full-width-dropdown mobile-full-width-dropdown-above-composer",
					"w-72 overflow-hidden border-border-default p-0",
					contentClassName,
				)}
				onOpenAutoFocus={(event) => {
					// On touch devices, auto-focusing the search input pops the
					// software keyboard as soon as the picker opens, hiding the
					// model list behind it. Only keep the WAI-ARIA combobox
					// focus-into-input behavior for fine pointers (keyboard and
					// mouse users on desktop).
					if (matchMedia("(pointer: coarse)").matches) {
						event.preventDefault();
					}
				}}
			>
				<Command
					shouldFilter={false}
					className="[&_[cmdk-input-wrapper]]:border-0 [&_[cmdk-input-wrapper]]:border-border-default [&_[cmdk-input-wrapper]]:border-b [&_[cmdk-input-wrapper]]:border-solid [&_[cmdk-input-wrapper]]:px-3 [&_[cmdk-input-wrapper]]:py-2 [&_[cmdk-input-wrapper]>svg]:size-3.5"
				>
					<CommandInput
						value={search}
						onValueChange={setSearch}
						placeholder="Search..."
						aria-label="Search models"
						className="h-auto py-0 text-xs font-normal leading-[18px] text-content-primary placeholder:text-content-disabled"
					/>
					<CommandList
						role="listbox"
						className={cn(
							"max-h-80 border-t-0",
							enableMobileFullWidthDropdown &&
								"mobile-full-width-dropdown-scroll-area",
						)}
					>
						<CommandEmpty className="py-3 text-xs font-normal leading-[18px] text-content-secondary">
							{emptyMessage}
						</CommandEmpty>
						{optionsByProvider.map(([provider, providerOptions], index) => (
							<CommandGroup
								key={provider}
								heading={formatProviderLabel(provider)}
								className={cn(
									"p-1 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:leading-[18px] [&_[cmdk-group-heading]]:text-content-secondary",
									index > 0 &&
										"border-0 border-t border-solid border-border-default",
								)}
							>
								{providerOptions.map((option) => (
									<ModelOptionItem
										key={option.id}
										option={option}
										isSelected={option.id === value}
										onSelect={() => {
											onValueChange(option.id);
											handleOpenChange(false);
										}}
									/>
								))}
							</CommandGroup>
						))}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

interface ModelOptionItemProps {
	option: ModelSelectorOption;
	isSelected: boolean;
	onSelect: () => void;
}

const ModelOptionItem: FC<ModelOptionItemProps> = ({
	option,
	isSelected,
	onSelect,
}) => {
	return (
		<CommandItem
			value={option.id}
			onSelect={onSelect}
			className={cn(
				"gap-2 px-2 py-1 font-medium text-content-secondary data-[selected=true]:bg-surface-tertiary",
				isSelected && "bg-surface-secondary",
			)}
		>
			<span className="min-w-0 truncate text-left text-xs font-medium leading-[18px] text-content-secondary">
				{option.displayName}
			</span>
			{option.contextLimit != null && option.contextLimit > 0 && (
				<span className="shrink-0 truncate text-left text-xs font-medium leading-[18px] text-content-secondary">
					({formatContextLimit(option.contextLimit)})
				</span>
			)}
			<CheckIcon
				className={cn("ml-auto size-4 shrink-0", !isSelected && "opacity-0")}
			/>
		</CommandItem>
	);
};
