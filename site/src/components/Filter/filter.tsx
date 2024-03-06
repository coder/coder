import { useTheme } from "@emotion/react";
import CheckOutlined from "@mui/icons-material/CheckOutlined";
import CloseOutlined from "@mui/icons-material/CloseOutlined";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import SearchOutlined from "@mui/icons-material/SearchOutlined";
import Button, { type ButtonProps } from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import IconButton from "@mui/material/IconButton";
import InputAdornment from "@mui/material/InputAdornment";
import Menu, { type MenuProps } from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import Skeleton, { type SkeletonProps } from "@mui/material/Skeleton";
import TextField from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import {
  type FC,
  type ReactNode,
  forwardRef,
  useEffect,
  useRef,
  useState,
} from "react";
import type { useSearchParams } from "react-router-dom";
import {
  getValidationErrorMessage,
  hasError,
  isApiValidationError,
} from "api/errors";
import { Loader } from "components/Loader/Loader";
import {
  Search,
  SearchEmpty,
  SearchInput,
  searchStyles,
} from "components/Menu/Search";
import { useDebouncedFunction } from "hooks/debounce";
import type { useFilterMenu } from "./menu";
import type { BaseOption } from "./options";

export type PresetFilter = {
  name: string;
  query: string;
};

type FilterValues = Record<string, string | undefined>;

type UseFilterConfig = {
  /**
   * The fallback value to use in the event that no filter params can be parsed
   * from the search params object. This value is allowed to change on
   * re-renders.
   */
  fallbackFilter?: string;
  searchParamsResult: ReturnType<typeof useSearchParams>;
  onUpdate?: (newValue: string) => void;
};

export const useFilterParamsKey = "filter";

export const useFilter = ({
  fallbackFilter = "",
  searchParamsResult,
  onUpdate,
}: UseFilterConfig) => {
  const [searchParams, setSearchParams] = searchParamsResult;
  const query = searchParams.get(useFilterParamsKey) ?? fallbackFilter;

  const update = (newValues: string | FilterValues) => {
    const serialized =
      typeof newValues === "string" ? newValues : stringifyFilter(newValues);

    searchParams.set(useFilterParamsKey, serialized);
    setSearchParams(searchParams);

    if (onUpdate !== undefined) {
      onUpdate(serialized);
    }
  };

  const { debounced: debounceUpdate, cancelDebounce } = useDebouncedFunction(
    update,
    500,
  );

  return {
    query,
    update,
    debounceUpdate,
    cancelDebounce,
    values: parseFilterQuery(query),
    used: query !== "" && query !== fallbackFilter,
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

const BaseSkeleton: FC<SkeletonProps> = ({ children, ...skeletonProps }) => {
  return (
    <Skeleton
      variant="rectangular"
      height={36}
      {...skeletonProps}
      css={(theme) => ({
        backgroundColor: theme.palette.background.paper,
        borderRadius: "6px",
      })}
    >
      {children}
    </Skeleton>
  );
};

export const SearchFieldSkeleton: FC = () => {
  return <BaseSkeleton width="100%" />;
};

export const MenuSkeleton: FC = () => {
  return <BaseSkeleton css={{ minWidth: 200, flexShrink: 0 }} />;
};

type FilterProps = {
  filter: ReturnType<typeof useFilter>;
  skeleton: ReactNode;
  isLoading: boolean;
  learnMoreLink: string;
  learnMoreLabel2?: string;
  learnMoreLink2?: string;
  error?: unknown;
  options?: ReactNode;
  presets: PresetFilter[];
};

export const Filter: FC<FilterProps> = ({
  filter,
  isLoading,
  error,
  skeleton,
  options,
  learnMoreLink,
  learnMoreLabel2,
  learnMoreLink2,
  presets,
}) => {
  const theme = useTheme();
  // Storing local copy of the filter query so that it can be updated more
  // aggressively without re-renders rippling out to the rest of the app every
  // single time. Exists for performance reasons - not really a good way to
  // remove this; render keys would cause the component to remount too often
  const [queryCopy, setQueryCopy] = useState(filter.query);
  const textboxInputRef = useRef<HTMLInputElement>(null);

  // Conditionally re-syncs the parent and local filter queries
  useEffect(() => {
    const hasSelfOrInnerFocus =
      textboxInputRef.current?.contains(document.activeElement) ?? false;

    // This doesn't address all state sync issues - namely, what happens if the
    // user removes focus just after this synchronizing effect fires. Also need
    // to rely on onBlur behavior as an extra safety measure
    if (!hasSelfOrInnerFocus) {
      setQueryCopy(filter.query);
    }
  }, [filter.query]);

  const shouldDisplayError = hasError(error) && isApiValidationError(error);
  const hasFilterQuery = filter.query !== "";

  return (
    <div
      css={{
        display: "flex",
        gap: 8,
        marginBottom: 16,
        flexWrap: "nowrap",

        [theme.breakpoints.down("md")]: {
          flexWrap: "wrap",
        },
      }}
    >
      {isLoading ? (
        skeleton
      ) : (
        <>
          <div css={{ display: "flex", width: "100%" }}>
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
                value: queryCopy,
                ref: textboxInputRef,
                onChange: (e) => {
                  setQueryCopy(e.target.value);
                  filter.debounceUpdate(e.target.value);
                },
                onBlur: () => {
                  if (queryCopy !== filter.query) {
                    setQueryCopy(filter.query);
                  }
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
                    color: theme.palette.text.secondary,
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
                      css={{
                        fontSize: 14,
                        color: theme.palette.text.secondary,
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
                        <CloseOutlined css={{ fontSize: 14 }} />
                      </IconButton>
                    </Tooltip>
                  </InputAdornment>
                ),
              }}
            />
          </div>
          {options}
        </>
      )}
    </div>
  );
};

interface PresetMenuProps {
  presets: PresetFilter[];
  learnMoreLink: string;
  learnMoreLabel2?: string;
  learnMoreLink2?: string;
  onSelect: (query: string) => void;
}

const PresetMenu: FC<PresetMenuProps> = ({
  presets,
  learnMoreLink,
  learnMoreLabel2,
  learnMoreLink2,
  onSelect,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);
  const theme = useTheme();

  return (
    <>
      <Button
        onClick={() => setIsOpen(true)}
        ref={anchorRef}
        css={{
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
        css={{ "& .MuiMenu-paper": { paddingTop: 8, paddingBottom: 8 } }}
      >
        {presets.map((presetFilter) => (
          <MenuItem
            css={{ fontSize: 14 }}
            key={presetFilter.name}
            onClick={() => {
              onSelect(presetFilter.query);
              setIsOpen(false);
            }}
          >
            {presetFilter.name}
          </MenuItem>
        ))}
        <Divider css={{ borderColor: theme.palette.divider }} />
        <MenuItem
          component="a"
          href={learnMoreLink}
          target="_blank"
          css={{ fontSize: 13, fontWeight: 500 }}
          onClick={() => {
            setIsOpen(false);
          }}
        >
          <OpenInNewOutlined css={{ fontSize: "14px !important" }} />
          View advanced filtering
        </MenuItem>
        {learnMoreLink2 && learnMoreLabel2 && (
          <MenuItem
            component="a"
            href={learnMoreLink2}
            target="_blank"
            css={{ fontSize: 13, fontWeight: 500 }}
            onClick={() => {
              setIsOpen(false);
            }}
          >
            <OpenInNewOutlined css={{ fontSize: "14px !important" }} />
            {learnMoreLabel2}
          </MenuItem>
        )}
      </Menu>
    </>
  );
};

interface FilterMenuProps<TOption extends BaseOption> {
  menu: ReturnType<typeof useFilterMenu<TOption>>;
  label: ReactNode;
  id: string;
  children: (values: { option: TOption; isSelected: boolean }) => ReactNode;
}

export const FilterMenu = <TOption extends BaseOption>(
  props: FilterMenuProps<TOption>,
) => {
  const { id, menu, label, children } = props;
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
        css={{ minWidth: 200 }}
      >
        {label}
      </MenuButton>
      <Menu
        id={id}
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        css={{ "& .MuiPaper-root": { minWidth: 200 } }}
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

interface FilterSearchMenuProps<TOption extends BaseOption> {
  menu: ReturnType<typeof useFilterMenu<TOption>>;
  label: ReactNode;
  id: string;
  children: (values: { option: TOption; isSelected: boolean }) => ReactNode;
}

export const FilterSearchMenu = <TOption extends BaseOption>({
  id,
  menu,
  label,
  children,
}: FilterSearchMenuProps<TOption>) => {
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
        css={{ minWidth: 200 }}
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
            key={option.value}
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

export const OptionItem: FC<OptionItemProps> = ({
  option,
  left,
  isSelected,
}) => {
  return (
    <div
      css={{
        display: "flex",
        alignItems: "center",
        gap: 16,
        fontSize: 14,
        overflow: "hidden",
        width: "100%",
      }}
    >
      {left}
      <span css={{ overflow: "hidden", textOverflow: "ellipsis" }}>
        {option.label}
      </span>
      {isSelected && (
        <CheckOutlined css={{ width: 16, height: 16, marginLeft: "auto" }} />
      )}
    </div>
  );
};

const MenuButton = forwardRef<HTMLButtonElement, ButtonProps>((props, ref) => {
  const { children, ...attrs } = props;

  return (
    <Button
      ref={ref}
      endIcon={<KeyboardArrowDown />}
      css={{
        borderRadius: "6px",
        justifyContent: "space-between",
        lineHeight: "120%",
      }}
      {...attrs}
    >
      {children}
    </Button>
  );
});

interface SearchMenuProps<TOption extends BaseOption>
  extends Pick<MenuProps, "anchorEl" | "open" | "onClose" | "id"> {
  options?: TOption[];
  renderOption: (option: TOption) => ReactNode;
  query: string;
  onQueryChange: (query: string) => void;
}

function SearchMenu<TOption extends BaseOption>({
  options,
  renderOption,
  query,
  onQueryChange,
  ...menuProps
}: SearchMenuProps<TOption>) {
  const menuListRef = useRef<HTMLUListElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  return (
    <Menu
      {...menuProps}
      onClose={(event, reason) => {
        menuProps.onClose && menuProps.onClose(event, reason);
        onQueryChange("");
      }}
      css={{
        "& .MuiPaper-root": searchStyles.content,
      }}
      // Disabled this so when we clear the filter and do some sorting in the
      // search items it does not look strange. Github removes exit transitions
      // on their filters as well.
      transitionDuration={{
        enter: 250,
        exit: 0,
      }}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "ArrowDown" && menuListRef.current) {
          const firstItem = menuListRef.current.firstChild as HTMLElement;
          firstItem.focus();
        }
      }}
    >
      <Search component="li">
        <SearchInput
          autoFocus
          value={query}
          ref={searchInputRef}
          onChange={(e) => {
            onQueryChange(e.target.value);
          }}
        />
      </Search>

      <li css={{ maxHeight: 480, overflowY: "auto" }}>
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
              <SearchEmpty />
            )
          ) : (
            <Loader size={14} />
          )}
        </MenuList>
      </li>
    </Menu>
  );
}
