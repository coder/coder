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

export type SelectFilterOption = {
  startIcon?: ReactNode;
  label: ReactNode;
  value: string;
};

export type SelectFilterProps = {
  options: SelectFilterOption[];
  onSelect: (option: SelectFilterOption | undefined) => void;
  selectedOption?: SelectFilterOption;
  placeholder: string;
  emptyText?: string;
  // Search props
  search?: string;
  onSearchChange?: (search: string) => void;
  searchPlaceholder?: string;
  searchAriaLabel?: string;
};

export const SelectFilter: FC<SelectFilterProps> = ({
  options,
  selectedOption,
  onSelect,
  onSearchChange,
  placeholder,
  searchAriaLabel,
  searchPlaceholder,
  emptyText,
  search,
}) => {
  const [open, setOpen] = useState(false);

  return (
    <SelectMenu open={open} onOpenChange={setOpen}>
      <SelectMenuTrigger>
        <SelectMenuButton startIcon={selectedOption?.startIcon}>
          {selectedOption?.label ?? placeholder}
        </SelectMenuButton>
      </SelectMenuTrigger>
      <SelectMenuContent>
        {onSearchChange && (
          <SelectMenuSearch
            value={search}
            onChange={onSearchChange}
            placeholder={searchPlaceholder}
            inputProps={{ "aria-label": searchAriaLabel }}
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
