import OpenInNew from "@mui/icons-material/OpenInNew";
import Button from "@mui/material/Button";
import Divider from "@mui/material/Divider";
import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import type { FC } from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { MenuIcon } from "components/Menu/MenuIcon";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import { docs } from "utils/docs";

type PresetOption = Readonly<{
  label: string;
  value: string;
}>;

const options: PresetOption[] = [
  { label: "My workspaces", value: "owner:me" },
  { label: "All workspaces", value: "" },
  { label: "Running workspaces", value: "status:running" },
  { label: "Failed workspaces", value: "status:failed" },
  { label: "Outdated workspaces", value: "outdated:true" },
  { label: "Dormant workspaces", value: "dormant:true" },
];

type PresetFilterMenuProps = {
  onSelect: (value: string) => void;
};

export const PresetFiltersMenu: FC<PresetFilterMenuProps> = ({ onSelect }) => {
  return (
    <Popover>
      {({ setIsOpen }) => {
        return (
          <>
            <PopoverTrigger>
              <Button endIcon={<DropdownArrow />}>Filters</Button>
            </PopoverTrigger>
            <PopoverContent>
              <MenuList dense>
                {options.map((option) => (
                  <MenuItem
                    key={option.value}
                    onClick={() => {
                      setIsOpen(false);
                      onSelect(option.value);
                    }}
                  >
                    {option.label}
                  </MenuItem>
                ))}
                <Divider />
                <MenuItem
                  component="a"
                  href={docs("/workspaces#workspace-filtering")}
                  target="_blank"
                  onClick={() => {
                    setIsOpen(false);
                  }}
                >
                  Learn advanced filtering
                  <MenuIcon>
                    <OpenInNew aria-label="Open in new tab" />
                  </MenuIcon>
                </MenuItem>
              </MenuList>
            </PopoverContent>
          </>
        );
      }}
    </Popover>
  );
};
