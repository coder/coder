import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import { useQuery } from "react-query";
import { users } from "api/queries/users";
import { Loader } from "components/Loader/Loader";
import { MenuButton } from "components/Menu/MenuButton";
import { MenuCheck } from "components/Menu/MenuCheck";
import { MenuSearch } from "components/Menu/MenuSearch";
import {
  PopoverContent,
  PopoverTrigger,
  usePopover,
  withPopover,
} from "components/Popover/Popover";
import { UserAvatar } from "components/UserAvatar/UserAvatar";

type UserMenuProps = {
  placeholder: string;
  selected: string | undefined;
  onSelect: (value: string) => void;
};

export const UserMenu = withPopover<UserMenuProps>((props) => {
  const popover = usePopover();
  const { selected, onSelect } = props;
  const userOptionsQuery = useQuery({
    ...users({}),
    enabled: selected !== undefined || popover.isOpen,
  });
  const options = userOptionsQuery.data?.users.map((u) => ({
    label: u.name ?? u.username,
    value: u.id,
    avatar: <UserAvatar size="xs" username={u.username} src={u.avatar_url} />,
  }));
  const selectedOption = options?.find((option) => option.value === selected);

  return (
    <>
      <PopoverTrigger>
        <MenuButton
          aria-label="Select user"
          startIcon={<span>{selectedOption?.avatar}</span>}
        >
          {selectedOption ? selectedOption.label : "All users"}
        </MenuButton>
      </PopoverTrigger>
      <PopoverContent>
        {options ? (
          <>
            <MenuSearch
              id="user-search"
              label="Search user"
              placeholder="Search user..."
              value=""
              onChange={() => {}}
              autoFocus
            />
            <MenuList dense>
              {options.map((option) => {
                const isSelected = option.value === selected;

                return (
                  <MenuItem
                    autoFocus={isSelected}
                    selected={isSelected}
                    key={option.value}
                    onClick={() => {
                      popover.setIsOpen(false);
                      onSelect(option.value);
                    }}
                  >
                    {option.avatar}
                    {option.label}
                    <MenuCheck isVisible={isSelected} />
                  </MenuItem>
                );
              })}
            </MenuList>
          </>
        ) : (
          <Loader />
        )}
      </PopoverContent>
    </>
  );
});
