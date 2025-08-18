import MenuItem, { type MenuItemProps } from "@mui/material/MenuItem";
import MenuList, { type MenuListProps } from "@mui/material/MenuList";
import { Button, type ButtonProps } from "components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
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

const SIDE_PADDING = 16;

export const SelectMenu = Popover;

export const SelectMenuTrigger = PopoverTrigger;

export const SelectMenuContent = PopoverContent;

type SelectMenuButtonProps = ButtonProps & {
	startIcon?: React.ReactNode;
};

export const SelectMenuButton = forwardRef<
	HTMLButtonElement,
	SelectMenuButtonProps
>((props, ref) => {
	const { startIcon, ...restProps } = props;
	return (
		<Button variant="outline" size="lg" ref={ref} {...restProps}>
			{startIcon}
			<span className="text-left block overflow-hidden text-ellipsis flex-grow">
				{props.children}
			</span>
			<ChevronDownIcon />
		</Button>
	);
});

export const SelectMenuSearch: FC<SearchFieldProps> = (props) => {
	return (
		<SearchField
			fullWidth
			size="medium"
			css={(theme) => ({
				borderBottom: `1px solid ${theme.palette.divider}`,
				"& input": {
					fontSize: 14,
				},
				"& fieldset": {
					border: 0,
					borderRadius: 0,
				},
				"& .MuiInputBase-root": {
					padding: `12px ${SIDE_PADDING}px`,
				},
				"& .MuiInputAdornment-positionStart": {
					marginRight: SIDE_PADDING,
				},
			})}
			{...props}
			inputProps={{ autoFocus: true, ...props.inputProps }}
		/>
	);
};

export const SelectMenuList: FC<MenuListProps> = ({ children, className, ...attrs }) => {
	const items = useMemo(() => {
		let items = Children.toArray(children);
		if (!items.every(isValidElement)) {
			throw new Error("SelectMenuList only accepts MenuItem children");
		}
		items = moveSelectedElementToFirst(
			items as ReactElement<MenuItemProps>[],
		);
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

export const SelectMenuIcon: FC<HTMLProps<HTMLDivElement>> = ({ children, className, ...attrs }) => {
	return <div className={cn("mr-4", className)} {...attrs}>{children}</div>;
};

export const SelectMenuItem: FC<MenuItemProps> = ({ children, className, selected, ...attrs }) => {
	return (
		<MenuItem className={cn("text-sm gap-0 leading-none py-3 px-4", className)} {...attrs}>
			{children}
			{selected && <CheckIcon className="size-icon-xs ml-auto" />}
		</MenuItem>
	);
};
