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
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Check, ChevronDown, CornerDownLeft, Info } from "lucide-react";
import { type FC, type KeyboardEventHandler, useState } from "react";
import { cn } from "utils/cn";
import { ExternalImage } from "../ExternalImage/ExternalImage";

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
	id?: string;
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
	id,
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

	const handleOpenChange = (newOpen: boolean) => {
		setManagedOpen(newOpen);
		onOpenChange?.(newOpen);
	};

	return (
		<Popover open={isOpen} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					id={id}
					variant="outline"
					aria-expanded={isOpen}
					className="w-full justify-between group"
				>
					<span className={cn(!value && "text-content-secondary")}>
						{optionsMap.get(value)?.displayName || value || placeholder}
					</span>
					<ChevronDown className="size-icon-sm text-content-secondary group-hover:text-content-primary" />
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-[var(--radix-popover-trigger-width)]">
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
										// Close the popover after selection
										handleOpenChange(false);
									}}
								>
									{showIcons &&
										(option.icon ? (
											<ExternalImage
												className="w-4 h-4 object-contain"
												src={option.icon}
												alt=""
											/>
										) : (
											/* Placeholder for missing icon to maintain layout consistency */
											<div className="w-4 h-4"></div>
										))}
									{option.displayName}
									<div className="flex flex-row items-center ml-auto gap-1">
										{value === option.value && (
											<Check className="size-icon-sm" />
										)}
										{option.description && (
											<Tooltip>
												<TooltipTrigger asChild>
													<span
														className="flex"
														onMouseEnter={(e) => e.stopPropagation()}
													>
														<Info className="w-3.5 h-3.5 text-content-secondary" />
													</span>
												</TooltipTrigger>
												<TooltipContent side="right" sideOffset={10}>
													{option.description}
												</TooltipContent>
											</Tooltip>
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
