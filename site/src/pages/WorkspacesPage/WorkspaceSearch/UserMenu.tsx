import { css } from "@emotion/css";
import { useTheme } from "@emotion/react";
import CheckOutlined from "@mui/icons-material/CheckOutlined";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { users } from "api/queries/users";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Loader } from "components/Loader/Loader";
import {
  PopoverContent,
  PopoverTrigger,
  usePopover,
  withPopover,
} from "components/Popover/Popover";
import { SearchField } from "components/Search/SearchField";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";

type Option = {
  label: string;
  value: string;
  addon: ReactNode;
};

type SelectLabelProps = {
  option: Option;
  selected: boolean;
};

const SelectLabel: FC<SelectLabelProps> = ({ option, selected }) => {
  return (
    <Stack
      direction="row"
      alignItems="center"
      spacing={2}
      css={{ width: "100%", lineHeight: 1 }}
    >
      <span css={{ flexShrink: 0 }} role="presentation">
        {option.addon}
      </span>
      <span css={{ width: "100%" }}>{option.label}</span>
      <div css={{ width: 14, height: 14, flexShrink: 0 }} role="presentation">
        {selected && <CheckOutlined css={{ width: 14, height: 14 }} />}
      </div>
    </Stack>
  );
};

type UserMenuProps = {
  placeholder: string;
  selected: string | undefined;
  onSelect: (value: string) => void;
};

export const UserMenu = withPopover<UserMenuProps>((props) => {
  const theme = useTheme();
  const popover = usePopover();
  const { placeholder, selected, onSelect } = props;
  const userOptionsQuery = useQuery({
    ...users({}),
    enabled: selected !== undefined || popover.isOpen,
  });
  const options = userOptionsQuery.data?.users.map((u) => ({
    label: u.name ?? u.username,
    value: u.id,
    addon: <UserAvatar size="xs" username={u.username} src={u.avatar_url} />,
  }));
  const selectedOption = options?.find((option) => option.value === selected);

  return (
    <>
      <PopoverTrigger>
        <Button
          aria-label="Select user"
          endIcon={<DropdownArrow />}
          startIcon={<span>{selectedOption?.addon}</span>}
        >
          {selectedOption ? selectedOption.label : placeholder}
        </Button>
      </PopoverTrigger>
      <PopoverContent>
        {options ? (
          <>
            <SearchField
              id="search"
              label="Search user"
              value=""
              onChange={() => {}}
              className={css({
                "& fieldset": {
                  border: 0,
                  borderRadius: 0,
                  borderBottom: `1px solid ${theme.palette.divider}`,
                },
              })}
              InputProps={{
                autoFocus: true,
              }}
            />
            <MenuList dense>
              {options.map((option) => (
                <MenuItem
                  selected={option.value === selected}
                  key={option.value}
                  onClick={() => {
                    popover.setIsOpen(false);
                    onSelect(option.value);
                  }}
                >
                  <SelectLabel
                    option={option}
                    selected={option.value === selected}
                  />
                </MenuItem>
              ))}
            </MenuList>
          </>
        ) : (
          <Loader />
        )}
      </PopoverContent>
    </>
  );
});
