import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import { useState } from "react";
import { useQuery } from "react-query";
import { templates } from "api/queries/templates";
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
import { TemplateAvatar } from "components/TemplateAvatar/TemplateAvatar";

type TemplateMenuProps = {
  organizationId: string;
  selected: string | undefined;
  onSelect: (value: string) => void;
};

export const TemplateMenu = withPopover<TemplateMenuProps>((props) => {
  const popover = usePopover();
  const { organizationId, selected, onSelect } = props;
  const [filter, setFilter] = useState("");
  const templateOptionsQuery = useQuery({
    ...templates(organizationId),
    enabled: selected !== undefined || popover.isOpen,
  });
  const options = templateOptionsQuery.data
    ?.filter((t) => {
      const f = filter.toLowerCase();
      return (
        t.name?.toLowerCase().includes(f) ||
        t.display_name.toLowerCase().includes(f)
      );
    })
    .map((t) => ({
      label: t.display_name ?? t.name,
      value: t.id,
      avatar: <TemplateAvatar size="xs" template={t} />,
    }));
  const selectedOption = options?.find((option) => option.value === selected);

  return (
    <>
      <PopoverTrigger>
        <MenuButton
          aria-label="Select template"
          startIcon={<span>{selectedOption?.avatar}</span>}
        >
          {selectedOption ? selectedOption.label : "All templates"}
        </MenuButton>
      </PopoverTrigger>
      <PopoverContent>
        <MenuSearch
          id="template-search"
          label="Search template"
          placeholder="Search template..."
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
          <Loader />
        )}
      </PopoverContent>
    </>
  );
});
