import { ReactNode, forwardRef, useEffect, useRef, useState } from "react";
import Box from "@mui/material/Box";
import TextField from "@mui/material/TextField";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import Button, { ButtonProps } from "@mui/material/Button";
import Menu, { MenuProps } from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import SearchOutlined from "@mui/icons-material/SearchOutlined";
import InputAdornment from "@mui/material/InputAdornment";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import CloseOutlined from "@mui/icons-material/CloseOutlined";
import { useSearchParams } from "react-router-dom";
import Skeleton, { SkeletonProps } from "@mui/material/Skeleton";
import CheckOutlined from "@mui/icons-material/CheckOutlined";
import {
  getValidationErrorMessage,
  hasError,
  isApiValidationError,
} from "api/errors";
import { useFilterMenu } from "./menu";
import { BaseOption } from "./options";
import debounce from "just-debounce-it";
import MenuList from "@mui/material/MenuList";
import { Loader } from "components/Loader/Loader";
import Divider from "@mui/material/Divider";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";

export type PresetFilter = {
  name: string;
  query: string;
};

type FilterValues = Record<string, string | undefined>;

export const useFilter = ({
  initialValue = "",
  onUpdate,
  searchParamsResult,
}: {
  initialValue?: string;
  searchParamsResult: ReturnType<typeof useSearchParams>;
  onUpdate?: () => void;
}) => {
  const [searchParams, setSearchParams] = searchParamsResult;
  const query = searchParams.get("filter") ?? initialValue;
  const values = parseFilterQuery(query);

  const update = (values: string | FilterValues) => {
    if (typeof values === "string") {
      searchParams.set("filter", values);
    } else {
      searchParams.set("filter", stringifyFilter(values));
    }
    setSearchParams(searchParams);
    if (onUpdate) {
      onUpdate();
    }
  };

  const debounceUpdate = debounce(
    (values: string | FilterValues) => update(values),
    500,
  );

  const used = query !== "" && query !== initialValue;

  return {
    query,
    update,
    debounceUpdate,
    values,
    used,
  };
};

export type UseFilterResult = ReturnType<typeof useFilter>;

const parseFilterQuery = (filterQuery: string): FilterValues => {
  if (filterQuery === "") {
    return {};
  }

  const pairs = filterQuery.split(" ");
  const result: FilterValues = {};

  for (const pair of pairs) {
    const [key, value] = pair.split(":") as [
      keyof FilterValues,
      string | undefined,
    ];
    if (value) {
      result[key] = value;
    }
  }

  return result;
};

const stringifyFilter = (filterValue: FilterValues): string => {
  let result = "";

  for (const key in filterValue) {
    const value = filterValue[key];
    if (value) {
      result += `${key}:${value} `;
    }
  }

  return result.trim();
};

const BaseSkeleton = (props: SkeletonProps) => {
  return (
    <Skeleton
      variant="rectangular"
      height={36}
      {...props}
      sx={{
        bgcolor: (theme) => theme.palette.background.paperLight,
        borderRadius: "6px",
        ...props.sx,
      }}
    />
  );
};

export const SearchFieldSkeleton = () => <BaseSkeleton width="100%" />;
export const MenuSkeleton = () => (
  <BaseSkeleton sx={{ minWidth: 200, flexShrink: 0 }} />
);

export const Filter = ({
  filter,
  isLoading,
  error,
  skeleton,
  options,
  learnMoreLink,
  learnMoreLabel2,
  learnMoreLink2,
  presets,
}: {
  filter: ReturnType<typeof useFilter>;
  skeleton: ReactNode;
  isLoading: boolean;
  learnMoreLink: string;
  learnMoreLabel2?: string;
  learnMoreLink2?: string;
  error?: unknown;
  options?: ReactNode;
  presets: PresetFilter[];
}) => {
  const shouldDisplayError = hasError(error) && isApiValidationError(error);
  const hasFilterQuery = filter.query !== "";
  const [searchQuery, setSearchQuery] = useState(filter.query);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // We don't want to update this while the user is typing something or has the focus in the input
    const isFocused = document.activeElement === inputRef.current;
    if (!isFocused) {
      setSearchQuery(filter.query);
    }
  }, [filter.query]);

  return (
    <Box
      sx={{
        display: "flex",
        flexWrap: ["wrap", undefined, "nowrap"],
        gap: 1,
        mb: 2,
      }}
    >
      {isLoading ? (
        skeleton
      ) : (
        <>
          <Box sx={{ display: "flex", width: "100%" }}>
            <PresetMenu
              onSelect={(query) => filter.update(query)}
              presets={presets}
              learnMoreLink={learnMoreLink}
              learnMoreLabel2={learnMoreLabel2}
              learnMoreLink2={learnMoreLink2}
            />
            <TextField
              fullWidth
              error={shouldDisplayError}
              helperText={
                shouldDisplayError
                  ? getValidationErrorMessage(error)
                  : undefined
              }
              size="small"
              InputProps={{
                "aria-label": "Filter",
                name: "query",
                placeholder: "Search...",
                value: searchQuery,
                ref: inputRef,
                onChange: (e) => {
                  setSearchQuery(e.target.value);
                  filter.debounceUpdate(e.target.value);
                },
                sx: {
                  borderRadius: "6px",
                  borderTopLeftRadius: 0,
                  borderBottomLeftRadius: 0,
                  marginLeft: "-1px",
                  "&:hover": {
                    zIndex: 2,
                  },
                  "& input::placeholder": {
                    color: (theme) => theme.palette.text.secondary,
                  },
                  "& .MuiInputAdornment-root": {
                    marginLeft: 0,
                  },
                  "&.Mui-error": {
                    zIndex: 3,
                  },
                },
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchOutlined
                      sx={{
                        fontSize: 14,
                        color: (theme) => theme.palette.text.secondary,
                      }}
                    />
                  </InputAdornment>
                ),
                endAdornment: hasFilterQuery && (
                  <InputAdornment position="end">
                    <Tooltip title="Clear filter">
                      <IconButton
                        size="small"
                        onClick={() => {
                          filter.update("");
                        }}
                      >
                        <CloseOutlined sx={{ fontSize: 14 }} />
                      </IconButton>
                    </Tooltip>
                  </InputAdornment>
                ),
              }}
            />
          </Box>
          {options}
        </>
      )}
    </Box>
  );
};

const PresetMenu = ({
  presets,
  learnMoreLink,
  learnMoreLabel2,
  learnMoreLink2,
  onSelect,
}: {
  presets: PresetFilter[];
  learnMoreLink: string;
  learnMoreLabel2?: string;
  learnMoreLink2?: string;
  onSelect: (query: string) => void;
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);

  return (
    <>
      <Button
        onClick={() => setIsOpen(true)}
        ref={anchorRef}
        sx={{
          borderTopRightRadius: 0,
          borderBottomRightRadius: 0,
          flexShrink: 0,
          zIndex: 1,
        }}
        endIcon={<KeyboardArrowDown />}
      >
        Filters
      </Button>
      <Menu
        id="filter-menu"
        anchorEl={anchorRef.current}
        open={isOpen}
        onClose={() => setIsOpen(false)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
        sx={{ "& .MuiMenu-paper": { py: 1 } }}
      >
        {presets.map((presetFilter) => (
          <MenuItem
            sx={{ fontSize: 14 }}
            key={presetFilter.name}
            onClick={() => {
              onSelect(presetFilter.query);
              setIsOpen(false);
            }}
          >
            {presetFilter.name}
          </MenuItem>
        ))}
        <Divider sx={{ borderColor: (theme) => theme.palette.divider }} />
        <MenuItem
          component="a"
          href={learnMoreLink}
          target="_blank"
          sx={{ fontSize: 13, fontWeight: 500 }}
          onClick={() => {
            setIsOpen(false);
          }}
        >
          <OpenInNewOutlined sx={{ fontSize: "14px !important" }} />
          View advanced filtering
        </MenuItem>
        {learnMoreLink2 && learnMoreLabel2 && (
          <>
            <MenuItem
              component="a"
              href={learnMoreLink2}
              target="_blank"
              sx={{ fontSize: 13, fontWeight: 500 }}
              onClick={() => {
                setIsOpen(false);
              }}
            >
              <OpenInNewOutlined sx={{ fontSize: "14px !important" }} />
              {learnMoreLabel2}
            </MenuItem>
          </>
        )}
      </Menu>
    </>
  );
};

export const FilterMenu = <TOption extends BaseOption>({
  id,
  menu,
  label,
  children,
}: {
  menu: ReturnType<typeof useFilterMenu<TOption>>;
  label: ReactNode;
  id: string;
  children: (values: { option: TOption; isSelected: boolean }) => ReactNode;
}) => {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  const handleClose = () => {
    setIsMenuOpen(false);
  };

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        onClick={() => setIsMenuOpen(true)}
        sx={{ minWidth: 200 }}
      >
        {label}
      </MenuButton>
      <Menu
        id={id}
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        sx={{ "& .MuiPaper-root": { minWidth: 200 } }}
        // Disabled this so when we clear the filter and do some sorting in the
        // search items it does not look strange. Github removes exit transitions
        // on their filters as well.
        transitionDuration={{
          enter: 250,
          exit: 0,
        }}
      >
        {menu.searchOptions?.map((option) => (
          <MenuItem
            key={option.label}
            selected={option.value === menu.selectedOption?.value}
            onClick={() => {
              menu.selectOption(option);
              handleClose();
            }}
          >
            {children({
              option,
              isSelected: option.value === menu.selectedOption?.value,
            })}
          </MenuItem>
        ))}
      </Menu>
    </div>
  );
};

export const FilterSearchMenu = <TOption extends BaseOption>({
  id,
  menu,
  label,
  children,
}: {
  menu: ReturnType<typeof useFilterMenu<TOption>>;
  label: ReactNode;
  id: string;
  children: (values: { option: TOption; isSelected: boolean }) => ReactNode;
}) => {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  const handleClose = () => {
    setIsMenuOpen(false);
  };

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        onClick={() => setIsMenuOpen(true)}
        sx={{ minWidth: 200 }}
      >
        {label}
      </MenuButton>
      <SearchMenu
        id={id}
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        options={menu.searchOptions}
        query={menu.query}
        onQueryChange={menu.setQuery}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === menu.selectedOption?.value}
            onClick={() => {
              menu.selectOption(option);
              handleClose();
            }}
          >
            {children({
              option,
              isSelected: option.value === menu.selectedOption?.value,
            })}
          </MenuItem>
        )}
      />
    </div>
  );
};

type OptionItemProps = {
  option: BaseOption;
  left?: ReactNode;
  isSelected?: boolean;
};

export const OptionItem = ({ option, left, isSelected }: OptionItemProps) => {
  return (
    <Box
      display="flex"
      alignItems="center"
      gap={2}
      fontSize={14}
      overflow="hidden"
      width="100%"
    >
      {left}
      <Box component="span" overflow="hidden" textOverflow="ellipsis">
        {option.label}
      </Box>
      {isSelected && (
        <CheckOutlined sx={{ width: 16, height: 16, marginLeft: "auto" }} />
      )}
    </Box>
  );
};

const MenuButton = forwardRef<HTMLButtonElement, ButtonProps>((props, ref) => {
  return (
    <Button
      ref={ref}
      endIcon={<KeyboardArrowDown />}
      {...props}
      sx={{
        borderRadius: "6px",
        justifyContent: "space-between",
        lineHeight: "120%",
        ...props.sx,
      }}
    />
  );
});

function SearchMenu<TOption extends { label: string; value: string }>({
  options,
  renderOption,
  query,
  onQueryChange,
  ...menuProps
}: Pick<MenuProps, "anchorEl" | "open" | "onClose" | "id"> & {
  options?: TOption[];
  renderOption: (option: TOption) => ReactNode;
  query: string;
  onQueryChange: (query: string) => void;
}) {
  const menuListRef = useRef<HTMLUListElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  return (
    <Menu
      {...menuProps}
      onClose={(event, reason) => {
        menuProps.onClose && menuProps.onClose(event, reason);
        onQueryChange("");
      }}
      sx={{
        "& .MuiPaper-root": {
          width: 320,
          paddingY: 0,
        },
      }}
      // Disabled this so when we clear the filter and do some sorting in the
      // search items it does not look strange. Github removes exit transitions
      // on their filters as well.
      transitionDuration={{
        enter: 250,
        exit: 0,
      }}
    >
      <Box
        component="li"
        sx={{
          display: "flex",
          alignItems: "center",
          paddingLeft: 2,
          height: 40,
          borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
        }}
        onKeyDown={(e) => {
          e.stopPropagation();
          if (e.key === "ArrowDown" && menuListRef.current) {
            const firstItem = menuListRef.current.firstChild as HTMLElement;
            firstItem.focus();
          }
        }}
      >
        <SearchOutlined
          sx={{
            fontSize: 14,
            color: (theme) => theme.palette.text.secondary,
          }}
        />
        <Box
          tabIndex={-1}
          component="input"
          type="text"
          placeholder="Search..."
          autoFocus
          value={query}
          ref={searchInputRef}
          onChange={(e) => {
            onQueryChange(e.target.value);
          }}
          sx={{
            height: "100%",
            border: 0,
            background: "none",
            width: "100%",
            marginLeft: 2,
            outline: 0,
            "&::placeholder": {
              color: (theme) => theme.palette.text.secondary,
            },
          }}
        />
      </Box>

      <Box component="li" sx={{ maxHeight: 480, overflowY: "auto" }}>
        <MenuList
          ref={menuListRef}
          onKeyDown={(e) => {
            if (e.shiftKey && e.code === "Tab") {
              e.preventDefault();
              e.stopPropagation();
              searchInputRef.current?.focus();
            }
          }}
        >
          {options ? (
            options.length > 0 ? (
              options.map(renderOption)
            ) : (
              <Box
                sx={{
                  fontSize: 13,
                  color: (theme) => theme.palette.text.secondary,
                  textAlign: "center",
                  py: 1,
                }}
              >
                No results
              </Box>
            )
          ) : (
            <Loader size={14} />
          )}
        </MenuList>
      </Box>
    </Menu>
  );
}
