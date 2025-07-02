import * as React from "react";
import { useState, useRef, useEffect } from "react";
import { Check, ChevronDown, Search } from "lucide-react";
import { cn } from "utils/cn";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "components/Command/Command";

interface SearchableSelectProps {
	value?: string;
	onValueChange?: (value: string) => void;
	placeholder?: string;
	disabled?: boolean;
	required?: boolean;
	id?: string;
	children?: React.ReactNode;
	className?: string;
	emptyMessage?: string;
}

interface SearchableSelectTriggerProps {
	id?: string;
	children?: React.ReactNode;
	className?: string;
}

interface SearchableSelectContentProps {
	children?: React.ReactNode;
	className?: string;
}

interface SearchableSelectItemProps {
	value: string;
	children?: React.ReactNode;
	className?: string;
}

interface SearchableSelectValueProps {
	placeholder?: string;
	className?: string;
}

// Context to share state between compound components
interface SearchableSelectContextValue {
	value?: string;
	onValueChange?: (value: string) => void;
	open: boolean;
	setOpen: (open: boolean) => void;
	disabled?: boolean;
	placeholder?: string;
	items: Map<string, React.ReactNode>;
	setSearch: (search: string) => void;
	search: string;
	emptyMessage?: string;
}

const SearchableSelectContext = React.createContext<SearchableSelectContextValue | undefined>(undefined);

const useSearchableSelectContext = () => {
	const context = React.useContext(SearchableSelectContext);
	if (!context) {
		throw new Error("SearchableSelect components must be used within SearchableSelect");
	}
	return context;
};

export const SearchableSelect: React.FC<SearchableSelectProps> = ({
	value,
	onValueChange,
	placeholder = "Select option",
	disabled = false,
	required = false,
	id,
	children,
	className,
	emptyMessage = "No results found",
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const items = useRef(new Map<string, React.ReactNode>()).current;

	// Clear search when closing
	useEffect(() => {
		if (!open) {
			setSearch("");
		}
	}, [open]);

	const contextValue: SearchableSelectContextValue = {
		value,
		onValueChange,
		open,
		setOpen,
		disabled,
		placeholder,
		items,
		setSearch,
		search,
		emptyMessage,
	};

	return (
		<SearchableSelectContext.Provider value={contextValue}>
			<div className={className} data-required={required} id={id}>
				{children}
			</div>
		</SearchableSelectContext.Provider>
	);
};

export const SearchableSelectTrigger = React.forwardRef<
	HTMLButtonElement,
	SearchableSelectTriggerProps
>(({ id, children, className }, ref) => {
	const { open, setOpen, disabled } = useSearchableSelectContext();

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					ref={ref}
					id={id}
					type="button"
					role="combobox"
					aria-expanded={open}
					aria-haspopup="listbox"
					disabled={disabled}
					className={cn(
						`flex h-10 w-full font-medium items-center justify-between whitespace-nowrap rounded-md
            border border-border border-solid bg-transparent px-3 py-2 text-sm shadow-sm
            ring-offset-background text-content-secondary placeholder:text-content-secondary focus:outline-none
            focus:ring-2 focus:ring-content-link disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1
            focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link`,
						className
					)}
				>
					{children}
				</button>
			</PopoverTrigger>
		</Popover>
	);
});
SearchableSelectTrigger.displayName = "SearchableSelectTrigger";

export const SearchableSelectValue: React.FC<SearchableSelectValueProps> = ({
	placeholder,
	className,
}) => {
	const { value, items, placeholder: contextPlaceholder } = useSearchableSelectContext();
	const displayPlaceholder = placeholder || contextPlaceholder;

	return (
		<>
			<span className={cn(!value && "text-content-secondary", className)}>
				{value ? items.get(value) || value : displayPlaceholder}
			</span>
			<ChevronDown className="size-icon-sm cursor-pointer text-content-secondary hover:text-content-primary" />
		</>
	);
};

export const SearchableSelectContent: React.FC<SearchableSelectContentProps> = ({
	children,
	className,
}) => {
	const { setSearch, search, emptyMessage } = useSearchableSelectContext();

	return (
		<PopoverContent
			className={cn(
				"min-w-[var(--radix-popover-trigger-width)] p-0",
				className
			)}
		>
			<Command shouldFilter={false}>
				<div className="flex items-center border-b border-border px-3">
					<Search className="mr-2 size-icon-sm text-content-secondary" />
					<CommandInput
						placeholder="Search..."
						value={search}
						onValueChange={setSearch}
						className="flex h-10 w-full rounded-md bg-transparent py-3 text-sm outline-none placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-50"
					/>
				</div>
				<CommandList>
					<CommandEmpty className="py-6 text-center text-sm">
						{emptyMessage}
					</CommandEmpty>
					<CommandGroup className="p-1">
						{children}
					</CommandGroup>
				</CommandList>
			</Command>
		</PopoverContent>
	);
};

export const SearchableSelectItem: React.FC<SearchableSelectItemProps> = ({
	value,
	children,
	className,
}) => {
	const { value: selectedValue, onValueChange, setOpen, items, search } = useSearchableSelectContext();

	// Register item content
	useEffect(() => {
		items.set(value, children);
		return () => {
			items.delete(value);
		};
	}, [value, children, items]);

	// Simple search filter
	const searchableText = React.Children.toArray(children)
		.map(child => {
			if (typeof child === 'string') return child;
			if (React.isValidElement(child) && typeof child.props.children === 'string') {
				return child.props.children;
			}
			return '';
		})
		.join(' ')
		.toLowerCase();

	const isVisible = !search || searchableText.includes(search.toLowerCase()) || value.toLowerCase().includes(search.toLowerCase());

	if (!isVisible) {
		return null;
	}

	return (
		<CommandItem
			value={value}
			onSelect={() => {
				onValueChange?.(value);
				setOpen(false);
			}}
			className={cn(
				"relative flex w-full cursor-default select-none items-center rounded-sm py-1.5",
				"pl-2 pr-8 text-sm text-content-secondary outline-none focus:bg-surface-secondary",
				"focus:text-content-primary data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
				className
			)}
		>
			{children}
			{selectedValue === value && (
				<span className="absolute right-2 flex items-center justify-center">
					<Check className="size-icon-sm" />
				</span>
			)}
		</CommandItem>
	);
};
