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
import { Spinner } from "components/Spinner/Spinner";
import { Check, ChevronDown, X } from "lucide-react";
import {
	type KeyboardEvent,
	type ReactNode,
	useCallback,
	useState,
} from "react";
import { cn } from "utils/cn";

export interface AutocompleteOption {
	value: string;
	label: string;
}

export interface AutocompleteProps<TOption> {
	value: TOption | null;
	onChange: (value: TOption | null) => void;
	options: readonly TOption[];
	getOptionValue: (option: TOption) => string;
	getOptionLabel: (option: TOption) => string;
	isOptionEqualToValue?: (option: TOption, value: TOption) => boolean;
	renderOption?: (option: TOption, isSelected: boolean) => ReactNode;
	loading?: boolean;
	placeholder?: string;
	noOptionsText?: string;
	open?: boolean;
	onOpenChange?: (open: boolean) => void;
	inputValue?: string;
	onInputChange?: (value: string) => void;
	clearable?: boolean;
	disabled?: boolean;
	startAdornment?: ReactNode;
	className?: string;
	id?: string;
	"data-testid"?: string;
}

export function Autocomplete<TOption>({
	value,
	onChange,
	options,
	getOptionValue,
	getOptionLabel,
	isOptionEqualToValue,
	renderOption,
	loading = false,
	placeholder = "Select an option",
	noOptionsText = "No results found",
	open: controlledOpen,
	onOpenChange,
	inputValue: controlledInputValue,
	onInputChange,
	clearable = true,
	disabled = false,
	startAdornment,
	className,
	id,
	"data-testid": testId,
}: AutocompleteProps<TOption>) {
	const [managedOpen, setManagedOpen] = useState(false);
	const [managedInputValue, setManagedInputValue] = useState("");

	const isOpen = controlledOpen ?? managedOpen;
	const inputValue = controlledInputValue ?? managedInputValue;

	const handleOpenChange = useCallback(
		(newOpen: boolean) => {
			setManagedOpen(newOpen);
			onOpenChange?.(newOpen);
			if (!newOpen && controlledInputValue === undefined) {
				setManagedInputValue("");
			}
		},
		[onOpenChange, controlledInputValue],
	);

	const handleInputChange = useCallback(
		(newValue: string) => {
			setManagedInputValue(newValue);
			onInputChange?.(newValue);
		},
		[onInputChange],
	);

	const isSelected = useCallback(
		(option: TOption): boolean => {
			if (!value) return false;
			if (isOptionEqualToValue) {
				return isOptionEqualToValue(option, value);
			}
			return getOptionValue(option) === getOptionValue(value);
		},
		[value, isOptionEqualToValue, getOptionValue],
	);

	const handleSelect = useCallback(
		(option: TOption) => {
			if (isSelected(option) && clearable) {
				onChange(null);
			} else {
				onChange(option);
			}
			handleOpenChange(false);
		},
		[isSelected, clearable, onChange, handleOpenChange],
	);

	const handleClear = useCallback(
		(e: React.SyntheticEvent) => {
			e.stopPropagation();
			onChange(null);
			handleInputChange("");
		},
		[onChange, handleInputChange],
	);

	const handleKeyDown = useCallback(
		(e: KeyboardEvent<HTMLInputElement>) => {
			if (e.key === "Escape") {
				handleOpenChange(false);
			}
		},
		[handleOpenChange],
	);

	const displayValue = value ? getOptionLabel(value) : "";
	const showClearButton = clearable && value && !disabled;

	return (
		<Popover open={isOpen} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild disabled={disabled}>
				<button
					type="button"
					id={id}
					data-testid={testId}
					aria-expanded={isOpen}
					aria-haspopup="listbox"
					disabled={disabled}
					className={cn(
						`flex h-10 w-full items-center justify-between gap-2
						rounded-md border border-border border-solid bg-transparent px-3 py-2
						text-sm shadow-sm transition-colors
						placeholder:text-content-secondary
						focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
						disabled:cursor-not-allowed disabled:opacity-50`,
						className,
					)}
				>
					<span className="flex items-center gap-2 overflow-hidden flex-1 min-w-0">
						{startAdornment}
						<span
							className={cn(
								"truncate text-left",
								!displayValue && "text-content-secondary",
							)}
						>
							{displayValue || placeholder}
						</span>
					</span>
					<span className="flex items-center shrink-0">
						{loading && <Spinner size="sm" loading className="mr-1" />}
						{showClearButton && (
							<span
								role="button"
								tabIndex={0}
								onClick={handleClear}
								onKeyDown={(e) => {
									if (e.key === "Enter" || e.key === " ") {
										handleClear(e);
									}
								}}
								className="flex items-center justify-center size-5 rounded hover:bg-surface-secondary transition-colors"
								aria-label="Clear selection"
							>
								<X className="size-4 text-content-secondary hover:text-content-primary" />
							</span>
						)}
						<span className="flex items-center justify-center size-5">
							<ChevronDown
								className={cn(
									"size-4 text-content-secondary transition-transform",
									isOpen && "rotate-180",
								)}
							/>
						</span>
					</span>
				</button>
			</PopoverTrigger>
			<PopoverContent
				className="w-[var(--radix-popover-trigger-width)] p-0"
				align="start"
			>
				<Command shouldFilter={controlledInputValue === undefined}>
					<CommandInput
						placeholder={placeholder}
						value={inputValue}
						onValueChange={handleInputChange}
						onKeyDown={handleKeyDown}
					/>
					<CommandList>
						{loading ? (
							<div className="flex items-center justify-center py-6">
								<Spinner size="sm" loading />
							</div>
						) : (
							<>
								<CommandEmpty>{noOptionsText}</CommandEmpty>
								<CommandGroup>
									{options.map((option) => {
										const optionValue = getOptionValue(option);
										const optionLabel = getOptionLabel(option);
										const selected = isSelected(option);

										return (
											<CommandItem
												key={optionValue}
												value={optionValue}
												keywords={[optionLabel]}
												onSelect={() => handleSelect(option)}
												className="cursor-pointer"
											>
												{renderOption ? (
													renderOption(option, selected)
												) : (
													<>
														<span className="flex-1">{optionLabel}</span>
														{selected && <Check className="size-4 shrink-0" />}
													</>
												)}
											</CommandItem>
										);
									})}
								</CommandGroup>
							</>
						)}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
}
