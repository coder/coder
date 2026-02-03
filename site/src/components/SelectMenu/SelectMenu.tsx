import MenuItem, { type MenuItemProps } from "@mui/material/MenuItem";
import MenuList, { type MenuListProps } from "@mui/material/MenuList";
import { Button, type ButtonProps } from "components/Button/Button";
import {
	Popover,
	PopoverContent,
	type PopoverContentProps,
	PopoverTrigger,
	type PopoverTriggerProps,
} from "components/Popover/Popover";
import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import {
	Children,
	type FC,
	forwardRef,
	type HTMLProps,
	isValidElement,
	type ReactElement,
	useMemo,
} from "react";
import { cn } from "utils/cn";

export const SelectMenu = Popover;

export const SelectMenuTrigger: FC<PopoverTriggerProps> = (props) => {
	return <PopoverTrigger asChild {...props} />;
};

export const SelectMenuContent: FC<PopoverContentProps> = (props) => {
	return (
		<PopoverContent
			{...props}
			className={cn(
				"w-auto bg-surface-secondary border-surface-quaternary overflow-y-auto text-sm",
				props.className,
			)}
		/>
	);
};

type SelectMenuButtonProps = ButtonProps & {
	startIcon?: React.ReactNode;
};

export const SelectMenuButton = forwardRef<
	HTMLButtonElement,
	SelectMenuButtonProps
>(({ className, startIcon, children, ...props }, ref) => {
	return (
		<Button
			variant="outline"
			size="lg"
			ref={ref}
			// Shrink padding right slightly to account for visual weight of
			// the chevron
			className={cn("flex flex-row gap-2 pr-1.5", className)}
			{...props}
		>
			{startIcon}
			<span className="text-left block overflow-hidden text-ellipsis flex-grow">
				{children}
			</span>
			<ChevronDownIcon />
		</Button>
	);
});

export const SelectMenuSearch: FC<SearchFieldProps> = ({
	className,
	...props
}) => {
	return (
		<SearchField
			className={cn(
				"w-full border border-solid border-border [&_input]:text-sm",
				className,
			)}
			autoFocus={true}
			{...props}
		/>
	);
};

export const SelectMenuList: FC<MenuListProps> = ({
	children,
	className,
	...attrs
}) => {
	const items = useMemo(() => {
		let items = Children.toArray(children);
		if (!items.every(isValidElement)) {
			throw new Error("SelectMenuList only accepts MenuItem children");
		}
		items = moveSelectedElementToFirst(items as ReactElement<MenuItemProps>[]);
		return items;
	}, [children]);

	return (
		<MenuList className={cn("max-h-[480px]", className)} {...attrs}>
			{items}
		</MenuList>
	);
};

function moveSelectedElementToFirst(items: ReactElement<MenuItemProps>[]) {
	const selectedElement = items.find((i) => i.props.selected);
	if (!selectedElement) {
		return items;
	}
	const selectedElementIndex = items.indexOf(selectedElement);
	const newItems = items.slice();
	newItems.splice(selectedElementIndex, 1);
	newItems.unshift(selectedElement);
	return newItems;
}

export const SelectMenuIcon: FC<HTMLProps<HTMLDivElement>> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<div className={cn("mr-4", className)} {...attrs}>
			{children}
		</div>
	);
};

export const SelectMenuItem: FC<MenuItemProps> = ({
	children,
	className,
	selected,
	...attrs
}) => {
	return (
		<MenuItem
			className={cn("text-sm gap-0 leading-none py-3 px-4", className)}
			{...attrs}
		>
			{children}
			{selected && <CheckIcon className="size-icon-xs ml-auto" />}
		</MenuItem>
	);
};
