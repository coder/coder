import CheckOutlined from "@mui/icons-material/CheckOutlined";
import Button, { type ButtonProps } from "@mui/material/Button";
import MenuItem, { type MenuItemProps } from "@mui/material/MenuItem";
import MenuList, { type MenuListProps } from "@mui/material/MenuList";
import {
  type FC,
  forwardRef,
  Children,
  isValidElement,
  type HTMLProps,
} from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import {
  SearchField,
  type SearchFieldProps,
} from "components/SearchField/SearchField";

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
        "& input": {
          fontSize: 14,
        },

        "& fieldset": {
          border: 0,
          borderRadius: 0,
          borderBottom: `1px solid ${theme.palette.divider} !important`,
        },
        "& .MuiInputBase-root": {
          padding: `12px ${SIDE_PADDING}px`,
        },
        "& .MuiInputAdornment-positionStart": {
          marginRight: SIDE_PADDING,
        },
      })}
      {...props}
    />
  );
};

export const SelectMenuList: FC<MenuListProps> = (props) => {
  const items = Children.toArray(props.children);
  type ItemType = (typeof items)[number];
  const selectedAsFirst = (a: ItemType, b: ItemType) => {
    if (
      !isValidElement<MenuItemProps>(a) ||
      !isValidElement<MenuItemProps>(b)
    ) {
      throw new Error(
        "SelectMenuList children must be SelectMenuItem components",
      );
    }
    return a.props.selected ? -1 : 0;
  };
  items.sort(selectedAsFirst);
  return (
    <MenuList css={{ maxHeight: 480 }} {...props}>
      {items}
    </MenuList>
  );
};

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
