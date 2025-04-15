/**
 * This component is based on multiple-selector
 * @see {@link https://shadcnui-expansions.typeart.cc/docs/multiple-selector}
 */
import { Command as CommandPrimitive, useCommandState } from "cmdk";
import { Badge } from "components/Badge/Badge";
import {
	Command,
	CommandGroup,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import { useDebouncedValue } from "hooks/debounce";
import { ChevronDown, X } from "lucide-react";
import {
	type ComponentProps,
	type ComponentPropsWithoutRef,
	type KeyboardEvent,
	type ReactNode,
	forwardRef,
	useCallback,
	useEffect,
	useImperativeHandle,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

export interface Option {
	value: string;
	label: string;
	disable?: boolean;
	/** fixed option that can't be removed. */
	fixed?: boolean;
	/** Group the options by providing key. */
	[key: string]: string | boolean | undefined;
}
interface GroupOption {
	[key: string]: Option[];
}

interface MultiSelectComboboxProps {
	value?: Option[];
	defaultOptions?: Option[];
	/** manually controlled options */
	options?: Option[];
	placeholder?: string;
	/** Loading component. */
	loadingIndicator?: ReactNode;
	/** Empty component. */
	emptyIndicator?: ReactNode;
	/** Debounce time for async search. Only work with `onSearch`. */
	delay?: number;
	/**
	 * Only work with `onSearch` prop. Trigger search when `onFocus`.
	 * For example, when user click on the input, it will trigger the search to get initial options.
	 **/
	triggerSearchOnFocus?: boolean;
	/** async search */
	onSearch?: (value: string) => Promise<Option[]>;
	/**
	 * sync search. This search will not showing loadingIndicator.
	 * The rest props are the same as async search.
	 * i.e.: creatable, groupBy, delay.
	 **/
	onSearchSync?: (value: string) => Option[];
	onChange?: (options: Option[]) => void;
	/** Limit the maximum number of selected options. */
	maxSelected?: number;
	/** When the number of selected options exceeds the limit, the onMaxSelected will be called. */
	onMaxSelected?: (maxLimit: number) => void;
	/** Hide the placeholder when there are options selected. */
	hidePlaceholderWhenSelected?: boolean;
	disabled?: boolean;
	/** Group the options base on provided key. */
	groupBy?: string;
	className?: string;
	badgeClassName?: string;
	/**
	 * First item selected is a default behavior by cmdk. That is why the default is true.
	 * This is a workaround solution by add a dummy item.
	 *
	 * @see {@link https://github.com/pacocoursey/cmdk/issues/171}
	 */
	selectFirstItem?: boolean;
	/** Allow user to create option when there is no option matched. */
	creatable?: boolean;
	/** Props of `Command` */
	commandProps?: ComponentPropsWithoutRef<typeof Command>;
	/** Props of `CommandInput` */
	inputProps?: Omit<
		ComponentPropsWithoutRef<typeof CommandPrimitive.Input>,
		"value" | "placeholder" | "disabled"
	>;
	/** hide or show the button that clears all the selected options. */
	hideClearAllButton?: boolean;
}

export interface MultiSelectComboboxRef {
	selectedValue: Option[];
	input: HTMLInputElement;
	focus: () => void;
	reset: () => void;
}

function transitionToGroupOption(options: Option[], groupBy?: string) {
	if (options.length === 0) {
		return {};
	}
	if (!groupBy) {
		return {
			"": options,
		};
	}

	const groupOption: GroupOption = {};
	for (const option of options) {
		const key = (option[groupBy] as string) || "";
		if (!groupOption[key]) {
			groupOption[key] = [];
		}
		groupOption[key].push(option);
	}
	return groupOption;
}

function removePickedOption(groupOption: GroupOption, picked: Option[]) {
	const cloneOption = structuredClone(groupOption);

	for (const [key, value] of Object.entries(cloneOption)) {
		cloneOption[key] = value.filter(
			(val) => !picked.find((p) => p.value === val.value),
		);
	}
	return cloneOption;
}

function isOptionsExist(groupOption: GroupOption, targetOption: Option[]) {
	return Object.values(groupOption).some((value) =>
		value.some((option) => targetOption.some((o) => o.value === option.value)),
	);
}

/**
 * The `CommandEmpty` of shadcn/ui will cause the cmdk-empty to not render correctly.
 * Here a new CommandEmpty is created using the `Empty` implementation from `cmdk`.
 *
 * @reference: https://github.com/hsuanyi-chou/shadcn-ui-expansions/issues/34#issuecomment-1949561607
 **/
const CommandEmpty = forwardRef<
	HTMLDivElement,
	ComponentProps<typeof CommandPrimitive.Empty>
>(({ className, ...props }, forwardedRef) => {
	const render = useCommandState((state) => state.filtered.count === 0);

	if (!render) return null;

	return (
		<div
			ref={forwardedRef}
			className={cn("py-6 text-center text-sm", className)}
			cmdk-empty=""
			role="presentation"
			{...props}
		/>
	);
});

export const MultiSelectCombobox = forwardRef<
	MultiSelectComboboxRef,
	MultiSelectComboboxProps
>(
	(
		{
			value,
			onChange,
			placeholder,
			defaultOptions: arrayDefaultOptions = [],
			options: arrayOptions,
			delay,
			onSearch,
			onSearchSync,
			loadingIndicator,
			emptyIndicator,
			maxSelected = Number.MAX_SAFE_INTEGER,
			onMaxSelected,
			hidePlaceholderWhenSelected,
			disabled,
			groupBy,
			className,
			badgeClassName,
			selectFirstItem = true,
			creatable = false,
			triggerSearchOnFocus = false,
			commandProps,
			inputProps,
			hideClearAllButton = false,
		}: MultiSelectComboboxProps,
		ref,
	) => {
		const inputRef = useRef<HTMLInputElement>(null);
		const [open, setOpen] = useState(false);
		const [onScrollbar, setOnScrollbar] = useState(false);
		const [isLoading, setIsLoading] = useState(false);
		const dropdownRef = useRef<HTMLDivElement>(null);

		const [selected, setSelected] = useState<Option[]>(
			() => {
				if (arrayDefaultOptions && arrayDefaultOptions.length > 0) {
					return arrayDefaultOptions;
				}
				return [];
			}
		);
		const [options, setOptions] = useState<GroupOption>(
			transitionToGroupOption(arrayDefaultOptions, groupBy),
		);
		const [inputValue, setInputValue] = useState("");
		const debouncedSearchTerm = useDebouncedValue(inputValue, delay || 500);

		const [previousValue, setPreviousValue] = useState<Option[]>(value || []);
		if (value && value !== previousValue) {
			setPreviousValue(value);
			setSelected(value);
		}

		useImperativeHandle(
			ref,
			() => ({
				selectedValue: [...selected],
				input: inputRef.current as HTMLInputElement,
				focus: () => inputRef?.current?.focus(),
				reset: () => setSelected([]),
			}),
			[selected],
		);

		const handleUnselect = useCallback(
			(option: Option) => {
				const newOptions = selected.filter((s) => s.value !== option.value);
				setSelected(newOptions);
				onChange?.(newOptions);
			},
			[onChange, selected],
		);

		const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
			const input = inputRef.current;
			if (input) {
				if (e.key === "Delete" || e.key === "Backspace") {
					if (input.value === "" && selected.length > 0) {
						const lastSelectOption = selected[selected.length - 1];
						// If last item is fixed, we should not remove it.
						if (!lastSelectOption.fixed) {
							handleUnselect(selected[selected.length - 1]);
						}
					}
				}
				// This is not a default behavior of the <input /> field
				if (e.key === "Escape") {
					input.blur();
				}
			}
		};

		useEffect(() => {
			if (!open) {
				return;
			}

			const handleClickOutside = (event: MouseEvent | TouchEvent) => {
				if (
					dropdownRef.current &&
					!dropdownRef.current.contains(event.target as Node) &&
					inputRef.current &&
					!inputRef.current.contains(event.target as Node)
				) {
					setOpen(false);
					inputRef.current.blur();
				}
			};

			if (open) {
				document.addEventListener("mousedown", handleClickOutside);
				document.addEventListener("touchend", handleClickOutside);
			}

			return () => {
				document.removeEventListener("mousedown", handleClickOutside);
				document.removeEventListener("touchend", handleClickOutside);
			};
		}, [open]);

		useEffect(() => {
			/** If `onSearch` is provided, do not trigger options updated. */
			if (!arrayOptions || onSearch) {
				return;
			}
			const newOption = transitionToGroupOption(arrayOptions || [], groupBy);
			if (JSON.stringify(newOption) !== JSON.stringify(options)) {
				setOptions(newOption);
			}
		}, [arrayOptions, groupBy, onSearch, options]);

		useEffect(() => {
			/** sync search */

			const doSearchSync = () => {
				const res = onSearchSync?.(debouncedSearchTerm);
				setOptions(transitionToGroupOption(res || [], groupBy));
			};

			const exec = () => {
				if (!onSearchSync || !open) return;

				if (triggerSearchOnFocus) {
					doSearchSync();
				}

				if (debouncedSearchTerm) {
					doSearchSync();
				}
			};

			void exec();
		}, [
			debouncedSearchTerm,
			groupBy,
			open,
			triggerSearchOnFocus,
			onSearchSync,
		]);

		useEffect(() => {
			/** async search */

			const doSearch = async () => {
				setIsLoading(true);
				const res = await onSearch?.(debouncedSearchTerm);
				setOptions(transitionToGroupOption(res || [], groupBy));
				setIsLoading(false);
			};

			const exec = async () => {
				if (!onSearch || !open) return;

				if (triggerSearchOnFocus) {
					await doSearch();
				}

				if (debouncedSearchTerm) {
					await doSearch();
				}
			};

			void exec();
		}, [debouncedSearchTerm, groupBy, open, triggerSearchOnFocus, onSearch]);

		const CreatableItem = () => {
			if (!creatable) return undefined;
			if (
				isOptionsExist(options, [{ value: inputValue, label: inputValue }]) ||
				selected.find((s) => s.value === inputValue)
			) {
				return undefined;
			}

			const Item = (
				<CommandItem
					value={inputValue}
					className="cursor-pointer"
					onMouseDown={(e) => {
						e.preventDefault();
						e.stopPropagation();
					}}
					onSelect={(value: string) => {
						if (selected.length >= maxSelected) {
							onMaxSelected?.(selected.length);
							return;
						}
						setInputValue("");
						const newOptions = [...selected, { value, label: value }];
						setSelected(newOptions);
						onChange?.(newOptions);
					}}
				>
					Create "{inputValue}"
				</CommandItem>
			);

			// For normal creatable
			if (!onSearch && inputValue.length > 0) {
				return Item;
			}

			// For async search creatable. avoid showing creatable item before loading at first.
			if (onSearch && debouncedSearchTerm.length > 0 && !isLoading) {
				return Item;
			}

			return undefined;
		};

		const EmptyItem = useCallback(() => {
			if (!emptyIndicator) return undefined;

			// For async search that showing emptyIndicator
			if (onSearch && !creatable && Object.keys(options).length === 0) {
				return (
					<CommandItem value="-" disabled>
						{emptyIndicator}
					</CommandItem>
				);
			}

			return <CommandEmpty>{emptyIndicator}</CommandEmpty>;
		}, [creatable, emptyIndicator, onSearch, options]);

		const selectables = useMemo<GroupOption>(
			() => removePickedOption(options, selected),
			[options, selected],
		);

		/** Avoid Creatable Selector freezing or lagging when paste a long string. */
		const commandFilter = () => {
			if (commandProps?.filter) {
				return commandProps.filter;
			}

			if (creatable) {
				return (value: string, search: string) => {
					return value.toLowerCase().includes(search.toLowerCase()) ? 1 : -1;
				};
			}
			// Using default filter in `cmdk`. We don't have to provide it.
			return undefined;
		};

		if (inputRef.current && inputProps?.id) {
			inputRef.current.id = inputProps?.id;
		}

		const fixedOptions = selected.filter((s) => s.fixed);

		return (
			<Command
				ref={dropdownRef}
				{...commandProps}
				onKeyDown={(e) => {
					handleKeyDown(e);
					commandProps?.onKeyDown?.(e);
				}}
				className={cn(
					"h-auto overflow-visible bg-transparent",
					commandProps?.className,
				)}
				shouldFilter={
					commandProps?.shouldFilter !== undefined
						? commandProps.shouldFilter
						: !onSearch
				} // When onSearch is provided, we don't want to filter the options. You can still override it.
				filter={commandFilter()}
			>
				{/* biome-ignore lint/a11y/useKeyWithClickEvents: onKeyDown is not needed here */}
				<div
					className={cn(
						`min-h-10 rounded-md border border-solid border-border text-sm pr-3
						focus-within:ring-2 focus-within:ring-content-link`,
						{
							"pl-3 py-1": selected.length !== 0,
							"cursor-text": !disabled && selected.length !== 0,
						},
						className,
					)}
					onClick={() => {
						if (disabled) return;
						inputRef?.current?.focus();
					}}
				>
					<div className="flex justify-between items-center">
						<div className="relative flex flex-wrap gap-1">
							{selected.map((option) => {
								return (
									<Badge
										key={option.value}
										className={cn(
											"data-[disabled]:bg-content-disabled data-[disabled]:text-surface-tertiarydata-[disabled]:hover:bg-content-disabled",
											"data-[fixed]:bg-content-disabled data-[fixed]:text-surface-tertiary data-[fixed]:hover:bg-surface-secondary",
											badgeClassName,
										)}
										data-fixed={option.fixed}
										data-disabled={disabled || undefined}
									>
										{option.label}
										<button
											type="button"
											data-testid="clear-option-button"
											className={cn(
												`ml-1 pr-0 rounded-sm bg-transparent border-none outline-none
												focus:ring-2 focus:ring-content-link focus:ml-2.5 focus:pl-0 cursor-pointer`,
												(disabled || option.fixed) && "hidden",
											)}
											onKeyDown={(e) => {
												if (e.key === "Enter") {
													handleUnselect(option);
												}
											}}
											onMouseDown={(e) => {
												e.preventDefault();
												e.stopPropagation();
											}}
											onClick={() => handleUnselect(option)}
										>
											<X className="h-4 w-4 text-content-secondary hover:text-content-primary align-text-bottom" />
										</button>
									</Badge>
								);
							})}
							{/* Avoid having the "Search" Icon */}
							<CommandPrimitive.Input
								{...inputProps}
								ref={inputRef}
								value={inputValue}
								disabled={disabled}
								onValueChange={(value) => {
									setInputValue(value);
									inputProps?.onValueChange?.(value);
								}}
								onBlur={(event) => {
									if (!onScrollbar) {
										setOpen(false);
									}
									inputProps?.onBlur?.(event);
								}}
								onFocus={(event) => {
									setOpen(true);
									triggerSearchOnFocus && onSearch?.(debouncedSearchTerm);
									inputProps?.onFocus?.(event);
								}}
								placeholder={
									hidePlaceholderWhenSelected && selected.length !== 0
										? ""
										: placeholder
								}
								className={cn(
									"flex-1 border-none outline-none bg-transparent placeholder:text-content-secondary",
									{
										"w-full": hidePlaceholderWhenSelected,
										"px-3 py-2.5": selected.length === 0,
										"ml-1": selected.length !== 0,
									},
									inputProps?.className,
								)}
							/>
						</div>
						<div className="flex items-center justify-between">
							<button
								type="button"
								data-testid="clear-all-button"
								onClick={() => {
									setSelected(fixedOptions);
									onChange?.(fixedOptions);
								}}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										setSelected(fixedOptions);
										onChange?.(fixedOptions);
									}
								}}
								className={cn(
									"bg-transparent mt-1 border-none rounded-sm cursor-pointer text-content-secondary hover:text-content-primary outline-none focus:ring-2 focus:ring-content-link",
									(hideClearAllButton ||
										disabled ||
										selected.length < 1 ||
										fixedOptions.length === selected.length) &&
										"hidden",
								)}
							>
								<X className="h-5 w-5" />
							</button>
							<ChevronDown className="size-icon-sm cursor-pointer text-content-secondary hover:text-content-primary" />
						</div>
					</div>
				</div>
				<div className="relative">
					{open && (
						<CommandList
							className={`absolute top-1 z-10 w-full rounded-md
								border border-solid border-border
								bg-surface-primary text-content-primary shadow-md outline-none
								animate-in`}
							onPointerLeave={() => {
								setOnScrollbar(false);
							}}
							onPointerEnter={() => {
								setOnScrollbar(true);
							}}
							onMouseUp={() => {
								inputRef?.current?.focus();
							}}
						>
							{isLoading ? (
								<>{loadingIndicator}</>
							) : (
								<>
									{EmptyItem()}
									{CreatableItem()}
									{!selectFirstItem && (
										<CommandItem value="-" className="hidden" />
									)}
									{Object.entries(selectables).map(([key, dropdowns]) => (
										<CommandGroup
											key={key}
											heading={key}
											className="h-full overflow-auto"
										>
											{/* biome-ignore lint/complexity/noUselessFragments: A parent element is
											    needed for multiple dropdown items */}
											<>
												{dropdowns.map((option) => {
													return (
														<CommandItem
															key={option.value}
															value={option.value}
															disabled={option.disable}
															onMouseDown={(e) => {
																e.preventDefault();
																e.stopPropagation();
															}}
															onSelect={() => {
																if (selected.length >= maxSelected) {
																	onMaxSelected?.(selected.length);
																	return;
																}
																setInputValue("");
																const newOptions = [...selected, option];
																setSelected(newOptions);
																onChange?.(newOptions);
															}}
															className={cn(
																"cursor-pointer",
																option.disable &&
																	"cursor-default text-content-disabled",
															)}
														>
															{option.label}
														</CommandItem>
													);
												})}
											</>
										</CommandGroup>
									))}
								</>
							)}
						</CommandList>
					)}
				</div>
			</Command>
		);
	},
);
