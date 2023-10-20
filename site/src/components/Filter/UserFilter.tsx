import { useMe } from "hooks";
import { BaseOption } from "./options";
import { getUsers } from "api/api";
import { UseFilterMenuOptions, useFilterMenu } from "./menu";
import { FilterSearchMenu, OptionItem } from "./filter";
import { UserAvatar } from "components/UserAvatar/UserAvatar";

export type UserOption = BaseOption & {
  avatarUrl?: string;
};

export const useUserFilterMenu = ({
  value,
  onChange,
  enabled,
}: Pick<
  UseFilterMenuOptions<UserOption>,
  "value" | "onChange" | "enabled"
>) => {
  const me = useMe();

  const addMeAsFirstOption = (options: UserOption[]) => {
    options = options.filter((option) => option.value !== me.username);
    return [
      { label: me.username, value: me.username, avatarUrl: me.avatar_url },
      ...options,
    ];
  };

  return useFilterMenu({
    onChange,
    enabled,
    value,
    id: "owner",
    getSelectedOption: async () => {
      if (value === "me") {
        return {
          label: me.username,
          value: me.username,
          avatarUrl: me.avatar_url,
        };
      }

      const usersRes = await getUsers({ q: value, limit: 1 });
      const firstUser = usersRes.users.at(0);
      if (firstUser && firstUser.username === value) {
        return {
          label: firstUser.username,
          value: firstUser.username,
          avatarUrl: firstUser.avatar_url,
        };
      }
      return null;
    },
    getOptions: async (query) => {
      const usersRes = await getUsers({ q: query, limit: 25 });
      let options: UserOption[] = usersRes.users.map((user) => ({
        label: user.username,
        value: user.username,
        avatarUrl: user.avatar_url,
      }));
      options = addMeAsFirstOption(options);
      return options;
    },
  });
};

export type UserFilterMenu = ReturnType<typeof useUserFilterMenu>;

export const UserMenu = (menu: UserFilterMenu) => {
  return (
    <FilterSearchMenu
      id="users-menu"
      menu={menu}
      label={
        menu.selectedOption ? (
          <UserOptionItem option={menu.selectedOption} />
        ) : (
          "All users"
        )
      }
    >
      {(itemProps) => <UserOptionItem {...itemProps} />}
    </FilterSearchMenu>
  );
};

const UserOptionItem = ({
  option,
  isSelected,
}: {
  option: UserOption;
  isSelected?: boolean;
}) => {
  return (
    <OptionItem
      option={option}
      isSelected={isSelected}
      left={
        <UserAvatar
          username={option.label}
          avatarURL={option.avatarUrl}
          sx={{ width: 16, height: 16, fontSize: 8 }}
        />
      }
    />
  );
};
