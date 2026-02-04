import { Button } from "components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandInput,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import type { SelectFilterOption } from "components/Filter/SelectFilter";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import type React from "react";
import { createContext, useContext, useState } from "react";
import { cn } from "utils/cn";

type ComboboxContextProps = {
	open: boolean;
	setOpen: (open: boolean) => void;
	value: string | undefined;
	onValueChange: ((value: string | undefined) => void) | undefined;
};

const ComboboxContext = createContext<ComboboxContextProps | null>(null);

function useCombobox() {
	const context = useContext(ComboboxContext);
	if (!context) {
		throw new Error("useCombobox must be used within a <Combobox />");
	}
	return context;
}

interface ComboboxProps extends React.ComponentProps<typeof Popover> {
	value?: string;
	onValueChange?: (value: string | undefined) => void;
}

function Combobox({
	children,
	open: controlledOpen,
	onOpenChange: controlledOnOpenChange,
	value,
	onValueChange,
	...props
}: ComboboxProps) {
	const [internalOpen, setInternalOpen] = useState(false);

	// Use controlled state if provided, otherwise use internal state
	const open = controlledOpen ?? internalOpen;
	const setOpen = controlledOnOpenChange ?? setInternalOpen;

	return (
		<ComboboxContext.Provider value={{ open, setOpen, value, onValueChange }}>
			<Popover open={open} onOpenChange={setOpen} {...props}>
				{children}
			</Popover>
		</ComboboxContext.Provider>
	);
}

const ComboboxTrigger = PopoverTrigger;

interface ComboboxButtonProps extends React.ComponentProps<"button"> {
	width?: number;
	selectedOption?: SelectFilterOption;
	placeholder?: string;
}

function ComboboxButton({
	children,
	className,
	width,
	selectedOption,
	placeholder,
	ref,
	...props
}: ComboboxButtonProps & { ref?: React.Ref<HTMLButtonElement> }) {
	return (
		<Button
			className="flex items-center justify-between shrink-0 grow gap-2 pr-1.5"
			style={{ flexBasis: width }}
			variant="outline"
			ref={ref}
			{...props}
		>
			{selectedOption?.startIcon}
			<span className="text-left block overflow-hidden text-ellipsis flex-grow">
				{selectedOption?.label ?? placeholder}
			</span>
			<ChevronDownIcon className="size-icon-sm" />
		</Button>
	);
}

function ComboboxContent({
	children,
	className,
	ref,
	...props
}: React.ComponentProps<typeof PopoverContent> & {
	ref?: React.Ref<HTMLDivElement>;
}) {
	return (
		<PopoverContent
			ref={ref}
			className={cn(
				"w-auto bg-surface-secondary border-surface-quaternary overflow-y-auto text-sm",
				className,
			)}
			{...props}
		>
			<Command className="bg-surface-secondary">{children}</Command>
		</PopoverContent>
	);
}

const ComboboxInput = CommandInput;
const ComboboxList = CommandList;

function ComboboxItem({
	children,
	className,
	onSelect,
	value,
	ref,
	...props
}: React.ComponentProps<typeof CommandItem> & {
	ref?: React.Ref<HTMLDivElement>;
}) {
	const { setOpen, value: selectedValue, onValueChange } = useCombobox();
	const isSelected = value === selectedValue;

	return (
		<CommandItem
			ref={ref}
			value={value}
			className={cn(className, "rounded-none")}
			onSelect={(itemValue) => {
				setOpen(false);
				// Toggle behavior: selecting the same value deselects it.
				const newValue = itemValue === selectedValue ? undefined : itemValue;
				onValueChange?.(newValue);
				onSelect?.(itemValue);
			}}
			{...props}
		>
			{children}
			<CheckIcon
				className={cn(
					"ml-2 h-4 w-4 min-w-0 flex-shrink-0",
					isSelected ? "opacity-100" : "opacity-0",
				)}
			/>
		</CommandItem>
	);
}
const ComboboxEmpty = CommandEmpty;

export {
	Combobox,
	ComboboxTrigger,
	ComboboxButton,
	ComboboxContent,
	ComboboxInput,
	ComboboxList,
	ComboboxItem,
	ComboboxEmpty,
};
