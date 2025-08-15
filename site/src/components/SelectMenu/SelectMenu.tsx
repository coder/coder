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
			className="border-solid border-b-px [&_input]:font-sm [&_fieldset]:border-none [&_fieldset]:rounded-none [&.MuiInputBase-root]:py-3 [&.MuiInputBase-root]:px-4 [&.MuiInputAdornment-positionStart]:mr-4"
			{...props}
			inputProps={{ autoFocus: true, ...props.inputProps }}
		/>
	);
};

export const SelectMenuList: FC<MenuListProps> = (props) => {
	const items = useMemo(() => {
		let children = Children.toArray(props.children);
		if (!children.every(isValidElement)) {
			throw new Error("SelectMenuList only accepts MenuItem children");
		}
		children = moveSelectedElementToFirst(
			children as ReactElement<MenuItemProps>[],
		);
		return children;
	}, [props.children]);
	return (
		<MenuList css={{ maxHeight: 480 }} {...props}>
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

export const SelectMenuIcon: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return <div css={{ marginRight: 16 }} {...props} />;
};

export const SelectMenuItem: FC<MenuItemProps> = (props) => {
	return (
		<MenuItem className="text-sm gap-0 leading-none py-3 px-4" {...props}>
			{props.children}
			{props.selected && <CheckIcon className="size-icon-xs ml-auto" />}
		</MenuItem>
	);
};
