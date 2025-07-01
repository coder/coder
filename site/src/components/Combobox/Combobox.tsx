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
import { Check, ChevronDown, CornerDownLeft } from "lucide-react";
import type { FC, KeyboardEventHandler } from "react";
import { cn } from "utils/cn";

interface ComboboxProps {
	value: string;
	options?: readonly string[];
	placeholder?: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	inputValue: string;
	onInputChange: (value: string) => void;
	onKeyDown?: KeyboardEventHandler<HTMLInputElement>;
	onSelect: (value: string) => void;
}

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
	return (
		<Popover open={open} onOpenChange={onOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="outline"
					aria-expanded={open}
					className="w-72 justify-between group"
				>
					<span className={cn(!value && "text-content-secondary")}>
						{value || placeholder}
					</span>
					<ChevronDown className="size-icon-sm text-content-secondary group-hover:text-content-primary" />
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-72">
				<Command>
					<CommandInput
						placeholder="Search or enter custom value"
						value={inputValue}
						onValueChange={onInputChange}
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
							{options.map((option) => (
								<CommandItem
									key={option}
									value={option}
									onSelect={(currentValue) => {
										onSelect(currentValue === value ? "" : currentValue);
									}}
								>
									{option}
									{value === option && (
										<Check className="size-icon-sm ml-auto" />
									)}
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
