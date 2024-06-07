import { type Theme, useTheme } from "@emotion/react";
import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import type { FC } from "react";
import { MenuButton } from "components/Menu/MenuButton";
import { MenuCheck } from "components/Menu/MenuCheck";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";

type StatusIndicatorProps = {
  color: keyof Theme["roles"];
};

const StatusIndicator: FC<StatusIndicatorProps> = ({ color }) => {
  const theme = useTheme();

  return (
    <div
      role="presentation"
      css={{
        width: 8,
        height: 8,
        borderRadius: "50%",
        backgroundColor: theme.roles[color].fill.outline,
      }}
    />
  );
};

type StatusOption = Readonly<{
  label: string;
  value: string;
  indicator: JSX.Element;
}>;

const options: StatusOption[] = [
  {
    label: "Running",
    value: "running",
    indicator: <StatusIndicator color="success" />,
  },
  {
    label: "Stopped",
    value: "stopped",
    indicator: <StatusIndicator color="inactive" />,
  },
  {
    label: "Failed",
    value: "failed",
    indicator: <StatusIndicator color="error" />,
  },
  {
    label: "Pending",
    value: "pending",
    indicator: <StatusIndicator color="info" />,
  },
];

type StatusMenuProps = {
  selected: string | undefined;
  onSelect: (value: string) => void;
};

export const StatusMenu: FC<StatusMenuProps> = (props) => {
  const { selected, onSelect } = props;
  const selectedOption = options.find((option) => option.value === selected);

  return (
    <Popover>
      {({ setIsOpen }) => {
        return (
          <>
            <PopoverTrigger>
              <MenuButton
                aria-label="Select status"
                startIcon={selectedOption?.indicator}
              >
                {selectedOption ? selectedOption.label : "All statuses"}
              </MenuButton>
            </PopoverTrigger>
            <PopoverContent>
              <MenuList dense>
                {options.map((option) => {
                  const isSelected = option.value === selected;

                  return (
                    <MenuItem
                      selected={isSelected}
                      key={option.value}
                      onClick={() => {
                        setIsOpen(false);
                        onSelect(option.value);
                      }}
                    >
                      {option.indicator}
                      {option.label}
                      <MenuCheck isVisible={isSelected} />
                    </MenuItem>
                  );
                })}
              </MenuList>
            </PopoverContent>
          </>
        );
      }}
    </Popover>
  );
};
