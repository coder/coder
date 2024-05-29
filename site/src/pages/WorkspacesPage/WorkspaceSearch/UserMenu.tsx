import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import { useState } from "react";
import { type QueryClient, useQuery, useQueryClient } from "react-query";
import { API } from "api/api";
import { meKey, usersKey, users as usersQuery } from "api/queries/users";
import type { User } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { MenuButton } from "components/Menu/MenuButton";
import { MenuCheck } from "components/Menu/MenuCheck";
import { MenuNoResults } from "components/Menu/MenuNoResults";
import { MenuSearch } from "components/Menu/MenuSearch";
import {
  PopoverContent,
  PopoverTrigger,
  usePopover,
  withPopover,
} from "components/Popover/Popover";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { useDebouncedValue } from "hooks/debounce";

type UserOption = {
  label: string;
  value: string;
  avatar: JSX.Element;
};

type UserMenuProps = {
  // The currently selected user email or undefined if no user is selected
  selected: UserOption["value"] | undefined;
  onSelect: (value: UserOption["value"]) => void;
};

export const UserMenu = withPopover<UserMenuProps>((props) => {
  const queryClient = useQueryClient();
  const popover = usePopover();
  const { selected, onSelect } = props;
  const [filter, setFilter] = useState("");
  const debouncedFilter = useDebouncedValue(filter, 300);
  const usersQueryResult = useQuery({
    ...usersQuery({ limit: 100, q: debouncedFilter }),
    enabled: popover.isOpen,
  });
  const { data: selectedUser } = useQuery({
    queryKey: selectedUserKey(selected ?? ""),
    queryFn: () => getSelectedUser(selected ?? "", queryClient),
    enabled: selected !== undefined,
  });
  const options = mountOptions(usersQueryResult.data?.users, selectedUser);
  const selectedOption = selectedUser
    ? optionFromUser(selectedUser)
    : undefined;

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
        <MenuSearch
          id="user-search"
          label="Search user"
          placeholder="Search user..."
          value={filter}
          onChange={setFilter}
          autoFocus
        />
        {options ? (
          options.length > 0 ? (
            <MenuList dense>
              {options.map((option) => {
                const isSelected = option.value === selected;

                return (
                  <MenuItem
                    autoFocus={isSelected}
                    selected={isSelected}
                    key={option.value}
                    onClick={() => {
                      const user = usersQueryResult.data?.users.find(
                        (u) => u.email === option.value,
                      );

                      if (!user) {
                        return;
                      }

                      // This avoid the need to refetch the selected user query
                      // when the user is selected
                      setSelectedUserQueryData(user, queryClient);
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
          ) : (
            <MenuNoResults />
          )
        ) : (
          <Loader size={20} />
        )}
      </PopoverContent>
    </>
  );
});

function selectedUserKey(email: string) {
  return usersKey({ limit: 1, q: email });
}

async function getSelectedUser(
  email: string,
  queryClient: QueryClient,
): Promise<User | undefined> {
  const loggedInUser = queryClient.getQueryData<User>(meKey);

  if (loggedInUser && loggedInUser.email === email) {
    return loggedInUser;
  }

  const usersRes = await API.getUsers({ q: email, limit: 1 });
  return usersRes.users.at(0);
}

function setSelectedUserQueryData(user: User, queryClient: QueryClient) {
  queryClient.setQueryData(selectedUserKey(user.email), user);
}

function optionFromUser(user: User): UserOption {
  return {
    label: user.name ?? user.username,
    value: user.email,
    avatar: (
      <UserAvatar size="xs" username={user.username} src={user.avatar_url} />
    ),
  };
}

function mountOptions(
  users: readonly User[] | undefined,
  selectedUser: User | undefined,
): UserOption[] | undefined {
  if (!users) {
    return undefined;
  }

  let usersToDisplay = [...users];

  if (selectedUser) {
    const usersIncludeSelectedUser = users.some(
      (u) => u.id === selectedUser.id,
    );
    if (!usersIncludeSelectedUser) {
      usersToDisplay = [selectedUser, ...usersToDisplay];
    }
  }

  return usersToDisplay.map(optionFromUser);
}
