import { CheckOutlined as CheckOutlined } from "lucide-react";
import Button, { type ButtonProps } from "@mui/material/Button";
import MenuItem, { type MenuItemProps } from "@mui/material/MenuItem";
import MenuList, { type MenuListProps } from "@mui/material/MenuList";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
	SearchField,
	type SearchFieldProps,
} from "components/SearchField/SearchField";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import {
	Children,
	type FC,
	type HTMLProps,
	type ReactElement,
	forwardRef,
	isValidElement,
	useMemo,
} from "react";

const SIDE_PADDING = 16;

export const SelectMenu = Popover;

export const SelectMenuTrigger = PopoverTrigger;

export const SelectMenuContent = PopoverContent;

export const SelectMenuButton = forwardRef<HTMLButtonElement, ButtonProps>(
	(props, ref) => {
		return (
			<Button
				css={{
					// Icon and text should be aligned to the left
					justifyContent: "flex-start",
					flexShrink: 0,
					"& .MuiButton-startIcon": {
						marginLeft: 0,
						marginRight: SIDE_PADDING,
					},
					// Dropdown arrow should be at the end of the button
					"& .MuiButton-endIcon": {
						marginLeft: "auto",
					},
				}}
				endIcon={<DropdownArrow />}
				ref={ref}
				{...props}
				// MUI applies a style that affects the sizes of start icons.
				// .MuiButton-startIcon > *:nth-of-type(1) { font-size: 20px }. To
				// prevent this from breaking the inner components of startIcon, we wrap
				// it in a div.
				startIcon={props.startIcon && <div>{props.startIcon}</div>}
			>
				<span
					// Make sure long text does not break the button layout
					css={{
						display: "block",
						overflow: "hidden",
						textOverflow: "ellipsis",
					}}
				>
					{props.children}
				</span>
			</Button>
		);
	},
);

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
		<MenuItem
			css={{
				fontSize: 14,
				gap: 0,
				lineHeight: 1,
				padding: `12px ${SIDE_PADDING}px`,
			}}
			{...props}
		>
			{props.children}
			{props.selected && (
				<CheckOutlined
					// TODO: Don't set the menu icon font size on default theme
					css={{ marginLeft: "auto", fontSize: "inherit !important" }}
				/>
			)}
		</MenuItem>
	);
};
