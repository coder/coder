import visuallyHidden from "@mui/utils/visuallyHidden";
import { useState, type FC, type ReactNode } from "react";
import { Loader } from "components/Loader/Loader";
import {
  SelectMenu,
  SelectMenuTrigger,
  SelectMenuButton,
  SelectMenuContent,
  SelectMenuSearch,
  SelectMenuList,
  SelectMenuItem,
  SelectMenuIcon,
} from "components/SelectMenu/SelectMenu";

const BASE_WIDTH = 200;

export type SelectFilterOption = {
  startIcon?: ReactNode;
  label: string;
  value: string;
};

export type SelectFilterProps = {
  options: SelectFilterOption[] | undefined;
  selectedOption?: SelectFilterOption;
  // Used to add a accessibility label to the select
  label: string;
  // Used when there is no option selected
  placeholder: string;
  // Used to customize the empty state message
  emptyText?: string;
  onSelect: (option: SelectFilterOption | undefined) => void;
  // Value of the search input
  search?: string;
  // Used to customize the search input placeholder
  searchPlaceholder?: string;
  // Used to add a accessibility label to the search input
  searchLabel?: string;
  onSearchChange?: (search: string) => void;
};

export const SelectFilter: FC<SelectFilterProps> = ({
  label,
  options,
  selectedOption,
  onSelect,
  onSearchChange,
  placeholder,
  searchLabel,
  searchPlaceholder,
  emptyText,
  search,
}) => {
  const [open, setOpen] = useState(false);

  return (
    <SelectMenu open={open} onOpenChange={setOpen}>
      <SelectMenuTrigger>
        <SelectMenuButton
          startIcon={selectedOption?.startIcon}
          css={{ width: BASE_WIDTH }}
        >
          {selectedOption?.label ?? placeholder}
          <span css={{ ...visuallyHidden }}>{label}</span>
        </SelectMenuButton>
      </SelectMenuTrigger>
      <SelectMenuContent
        horizontal="right"
        css={{
          "& .MuiPaper-root": {
            minWidth: BASE_WIDTH,
          },
        }}
      >
        {onSearchChange && (
          <SelectMenuSearch
            value={search}
            onChange={onSearchChange}
            placeholder={searchPlaceholder}
            inputProps={{ "aria-label": searchLabel }}
          />
        )}
        {options ? (
          options.length > 0 ? (
            <SelectMenuList>
              {options.map((o) => {
                const isSelected = o.value === selectedOption?.value;
                return (
                  <SelectMenuItem
                    key={o.value}
                    selected={isSelected}
                    onClick={() => {
                      setOpen(false);
                      onSelect(isSelected ? undefined : o);
                    }}
                  >
                    {o.startIcon && (
                      <SelectMenuIcon>{o.startIcon}</SelectMenuIcon>
                    )}
                    {o.label}
                  </SelectMenuItem>
                );
              })}
            </SelectMenuList>
          ) : (
            <div
              css={(theme) => ({
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                padding: 32,
                color: theme.palette.text.secondary,
                lineHeight: 1,
              })}
            >
              {emptyText ?? "No options found"}
            </div>
          )
        ) : (
          <Loader size={16} />
        )}
      </SelectMenuContent>
    </SelectMenu>
  );
};
