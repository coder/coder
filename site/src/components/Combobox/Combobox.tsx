import { Avatar } from "components/Avatar/Avatar";
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
	PopoverTrigger,
} from "components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Check, ChevronDown, CornerDownLeft } from "lucide-react";
import { Info } from "lucide-react";
import { type FC, type KeyboardEventHandler, useState } from "react";
import { cn } from "utils/cn";

interface ComboboxProps {
	value: string;
	options?: Readonly<Array<string | ComboboxOption>>;
	placeholder?: string;
	open?: boolean;
	onOpenChange?: (open: boolean) => void;
	inputValue?: string;
	onInputChange?: (value: string) => void;
	onKeyDown?: KeyboardEventHandler<HTMLInputElement>;
	onSelect: (value: string) => void;
}

type ComboboxOption = {
	icon?: string;
	displayName: string;
	value: string;
	description?: string;
};

export const Combobox: FC<ComboboxProps> = ({
	value,
	options = [],
	placeholder = "Select option",
	open,
	onOpenChange,
	inputValue,
	onInputChange,
	onKeyDown,
	onSelect,
}) => {
	const [managedOpen, setManagedOpen] = useState(false);
	const [managedInputValue, setManagedInputValue] = useState("");

	const optionsMap = new Map<string, ComboboxOption>(
		options.map((option) =>
			typeof option === "string"
				? [option, { displayName: option, value: option }]
				: [option.value, option],
		),
	);
	const optionObjects = [...optionsMap.values()];
	const showIcons = optionObjects.some((it) => it.icon);

	const isOpen = open ?? managedOpen;

	return (
		<Popover
			open={isOpen}
			onOpenChange={(newOpen) => {
				setManagedOpen(newOpen);
				onOpenChange?.(newOpen);
			}}
		>
			<PopoverTrigger asChild>
				<Button
					variant="outline"
					aria-expanded={isOpen}
					className="w-72 justify-between group"
				>
					<span className={cn(!value && "text-content-secondary")}>
						{optionsMap.get(value)?.displayName || value || placeholder}
					</span>
					<ChevronDown className="size-icon-sm text-content-secondary group-hover:text-content-primary" />
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-72">
				<Command>
					<CommandInput
						placeholder="Search or enter custom value"
						value={inputValue ?? managedInputValue}
						onValueChange={(newValue) => {
							setManagedInputValue(newValue);
							onInputChange?.(newValue);
						}}
						onKeyDown={onKeyDown}
					/>
					<CommandList>
						<CommandEmpty>
							<p>No results found</p>
							<span className="flex flex-row items-center justify-center gap-1">
								Enter custom value
								<CornerDownLeft className="size-icon-sm bg-surface-tertiary rounded-sm p-1" />
							</span>
						</CommandEmpty>
						<CommandGroup>
							{optionObjects.map((option) => (
								<CommandItem
									key={option.value}
									value={option.value}
									keywords={[option.displayName]}
									onSelect={(currentValue) => {
										onSelect(currentValue === value ? "" : currentValue);
									}}
								>
									{showIcons && (
										<Avatar
											size="sm"
											src={option.icon}
											fallback={option.value}
										/>
									)}
									{option.displayName}
									<div className="flex flex-row items-center ml-auto gap-1">
										{value === option.value && (
											<Check className="size-icon-sm" />
										)}
										{option.description && (
											<TooltipProvider delayDuration={100}>
												<Tooltip>
													<TooltipTrigger asChild>
														<Info className="w-3.5 h-3.5 text-content-secondary" />
													</TooltipTrigger>
													<TooltipContent side="right" sideOffset={10}>
														{option.description}
													</TooltipContent>
												</Tooltip>
											</TooltipProvider>
										)}
									</div>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
