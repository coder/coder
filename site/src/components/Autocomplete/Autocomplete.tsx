import { CheckIcon, XIcon } from "lucide-react";
import {
	type KeyboardEvent,
	type ReactNode,
	type SyntheticEvent,
	useCallback,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
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
	PopoverAnchor,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";

interface AutocompleteProps<TOption> {
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
	onEscapeKeyDown?: () => void;
	onEnterEmpty?: () => void;
	inlineSearch?: boolean;
	clearable?: boolean;
	disabled?: boolean;
	startAdornment?: ReactNode;
	className?: string;
	triggerAriaInvalid?: boolean;
	triggerAriaDescribedBy?: string;
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
	onEscapeKeyDown,
	onEnterEmpty,
	inlineSearch = false,
	clearable = true,
	disabled = false,
	startAdornment,
	className,
	triggerAriaInvalid,
	triggerAriaDescribedBy,
	id,
	"data-testid": testId,
}: AutocompleteProps<TOption>) {
	const inlineInputRef = useRef<HTMLInputElement>(null);
	const highlightedValueRef = useRef<string | null>(null);
	const [managedOpen, setManagedOpen] = useState(false);
	const [managedInputValue, setManagedInputValue] = useState("");
	const [highlightedValue, setHighlightedValue] = useState<string | null>(null);
	const generatedListboxId = useId();
	const listboxId = `${generatedListboxId}-listbox`;

	const updateHighlightedValue = useCallback((newValue: string | null) => {
		highlightedValueRef.current = newValue;
		setHighlightedValue(newValue);
	}, []);
	const isOpen = controlledOpen ?? managedOpen;
	const inputValue = controlledInputValue ?? managedInputValue;

	const handleOpenChange = useCallback(
		(newOpen: boolean) => {
			setManagedOpen(newOpen);
			onOpenChange?.(newOpen);
			if (!newOpen) {
				updateHighlightedValue(null);
			}
			if (!newOpen && controlledInputValue === undefined) {
				setManagedInputValue("");
			}
		},
		[onOpenChange, controlledInputValue, updateHighlightedValue],
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
		(e: SyntheticEvent) => {
			e.stopPropagation();
			onChange(null);
			handleInputChange("");
		},
		[onChange, handleInputChange],
	);

	const handleKeyDown = useCallback(
		(e: KeyboardEvent<HTMLElement>) => {
			if (e.key === "Escape") {
				// cmdk consumes Escape unless default is prevented before its handler.
				e.preventDefault();
				if (onEscapeKeyDown) {
					e.stopPropagation();
					onEscapeKeyDown();
				}
				handleOpenChange(false);
			}
		},
		[handleOpenChange, onEscapeKeyDown],
	);

	useEffect(() => {
		if (
			highlightedValue !== null &&
			!options.some((option) => getOptionValue(option) === highlightedValue)
		) {
			updateHighlightedValue(null);
		}
	}, [highlightedValue, options, getOptionValue, updateHighlightedValue]);

	const displayValue = value ? getOptionLabel(value) : "";
	const showClearButton = clearable && value && !disabled;
	const highlightedIndex = options.findIndex(
		(option) => getOptionValue(option) === highlightedValue,
	);
	const activeDescendant =
		highlightedIndex >= 0
			? `${listboxId}-option-${highlightedIndex}`
			: undefined;

	const handleInlineKeyDown = (e: KeyboardEvent<HTMLElement>) => {
		if (disabled) {
			return;
		}

		if (e.key === "ArrowDown" || e.key === "ArrowUp") {
			e.preventDefault();
			if (!isOpen) {
				handleOpenChange(true);
			}

			if (options.length === 0) {
				updateHighlightedValue(null);
				return;
			}

			const currentIndex = options.findIndex(
				(option) => getOptionValue(option) === highlightedValueRef.current,
			);
			const nextIndex =
				e.key === "ArrowDown"
					? (currentIndex + 1) % options.length
					: (currentIndex <= 0 ? options.length : currentIndex) - 1;
			const nextOption = options[nextIndex];
			if (!nextOption) {
				updateHighlightedValue(null);
				return;
			}
			updateHighlightedValue(getOptionValue(nextOption));
			return;
		}

		if (e.key === "Enter") {
			e.preventDefault();
			e.stopPropagation();
			if (!loading && options.length === 0) {
				onEnterEmpty?.();
				return;
			}

			const highlightedOption = options.find(
				(option) => getOptionValue(option) === highlightedValueRef.current,
			);
			if (highlightedOption) {
				handleSelect(highlightedOption);
			}
			return;
		}

		if (e.key === "Escape") {
			e.preventDefault();
			if (onEscapeKeyDown) {
				e.stopPropagation();
				onEscapeKeyDown();
			}
			handleOpenChange(false);
		}
	};

	const renderOptionContent = (option: TOption) => {
		const optionLabel = getOptionLabel(option);
		const selected = isSelected(option);

		return renderOption ? (
			renderOption(option, selected)
		) : (
			<>
				<span className="flex-1">{optionLabel}</span>
				{selected && <CheckIcon className="size-4 shrink-0" />}
			</>
		);
	};

	const isInlineInputTarget = (target: EventTarget | null) =>
		target instanceof Node &&
		inlineInputRef.current !== null &&
		inlineInputRef.current.contains(target);

	if (inlineSearch) {
		const inlineInputValue = isOpen ? inputValue : displayValue;
		const hasResults = loading || options.length > 0;
		const showPopover = isOpen && hasResults;

		return (
			<Popover open={showPopover} onOpenChange={handleOpenChange}>
				<PopoverAnchor asChild>
					<input
						ref={inlineInputRef}
						type="text"
						id={id}
						data-testid={testId}
						role="combobox"
						aria-expanded={showPopover}
						aria-controls={showPopover ? listboxId : undefined}
						aria-activedescendant={showPopover ? activeDescendant : undefined}
						aria-haspopup="listbox"
						aria-invalid={triggerAriaInvalid}
						aria-describedby={triggerAriaDescribedBy}
						disabled={disabled}
						placeholder={placeholder}
						value={inlineInputValue}
						onFocus={() => {
							if (!disabled && !isOpen) {
								handleOpenChange(true);
							}
						}}
						onMouseDown={() => {
							if (!disabled && !isOpen) {
								handleOpenChange(true);
							}
						}}
						onChange={(event) => {
							if (disabled) {
								return;
							}
							if (!isOpen) {
								handleOpenChange(true);
							}
							handleInputChange(event.currentTarget.value);
						}}
						onKeyDownCapture={handleInlineKeyDown}
						className={cn(
							`flex h-10 w-full items-center rounded-md border border-border border-solid
							bg-transparent px-3 py-2 text-sm shadow-sm transition-colors
							placeholder:text-content-secondary text-content-primary
							focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link
							disabled:cursor-not-allowed disabled:opacity-50`,
							className,
						)}
					/>
				</PopoverAnchor>
				<PopoverContent
					className="w-[var(--radix-popover-trigger-width)] p-0"
					align="start"
					onKeyDownCapture={handleInlineKeyDown}
					onOpenAutoFocus={(event) => event.preventDefault()}
					onCloseAutoFocus={(event) => event.preventDefault()}
					onInteractOutside={(event) => {
						if (isInlineInputTarget(event.target)) {
							event.preventDefault();
							return;
						}
						handleOpenChange(false);
					}}
				>
					<Command
						shouldFilter={false}
						value={highlightedValue ?? ""}
						onValueChange={(newValue) => {
							if (newValue) {
								updateHighlightedValue(newValue);
							}
						}}
					>
						<CommandList id={listboxId} role="listbox">
							{loading ? (
								<div className="flex items-center justify-center py-6">
									<Spinner size="sm" loading />
								</div>
							) : (
								<>
									<CommandEmpty>{noOptionsText}</CommandEmpty>
									<CommandGroup>
										{options.map((option, index) => {
											const optionValue = getOptionValue(option);

											return (
												<CommandItem
													role="option"
													id={`${listboxId}-option-${index}`}
													key={optionValue}
													value={optionValue}
													onSelect={() => handleSelect(option)}
													className="cursor-pointer"
												>
													{renderOptionContent(option)}
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

	return (
		<Popover open={isOpen} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild disabled={disabled}>
				<button
					type="button"
					id={id}
					data-testid={testId}
					aria-expanded={isOpen}
					aria-haspopup="listbox"
					aria-invalid={triggerAriaInvalid}
					aria-describedby={triggerAriaDescribedBy}
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
								displayValue
									? "text-content-primary"
									: "text-content-secondary",
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
								className="flex items-center justify-center size-5 rounded hover:bg-surface-secondary transition-colors cursor-pointer"
								aria-label="Clear selection"
							>
								<XIcon className="size-4 text-content-secondary hover:text-content-primary" />
							</span>
						)}
						<span className="flex items-center justify-center size-5">
							<ChevronDownIcon
								open={isOpen}
								className="size-4 text-content-secondary"
							/>
						</span>
					</span>
				</button>
			</PopoverTrigger>
			<PopoverContent
				className="w-[var(--radix-popover-trigger-width)] p-0"
				align="start"
				onKeyDownCapture={handleKeyDown}
			>
				<Command shouldFilter={controlledInputValue === undefined}>
					<CommandInput
						placeholder={placeholder}
						value={inputValue}
						onValueChange={handleInputChange}
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
														{selected && (
															<CheckIcon className="size-4 shrink-0" />
														)}
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
