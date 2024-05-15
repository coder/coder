import { type Theme, useTheme } from "@emotion/react";
import CheckOutlined from "@mui/icons-material/CheckOutlined";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import MenuList from "@mui/material/MenuList";
import type { FC, ReactNode } from "react";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import { Stack } from "components/Stack/Stack";

type StatusCircleProps = {
  color: keyof Theme["roles"];
};

const StatusCircle: FC<StatusCircleProps> = ({ color }) => {
  const theme = useTheme();

  return (
    <div
      css={{
        width: 8,
        height: 8,
        borderRadius: "50%",
        backgroundColor: theme.roles[color].fill.outline,
      }}
    />
  );
};

type Option = {
  label: string;
  value: string;
  addon: ReactNode;
};

const options: Option[] = [
  {
    label: "Running",
    value: "running",
    addon: <StatusCircle color="success" />,
  },
  {
    label: "Stopped",
    value: "stopped",
    addon: <StatusCircle color="inactive" />,
  },
  {
    label: "Failed",
    value: "failed",
    addon: <StatusCircle color="error" />,
  },
  {
    label: "Pending",
    value: "pending",
    addon: <StatusCircle color="info" />,
  },
];

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

type StatusMenuProps = {
  placeholder: string;
  selected: string | undefined;
  onSelect: (value: string) => void;
};

export const StatusMenu: FC<StatusMenuProps> = (props) => {
  const { placeholder, selected, onSelect } = props;
  const selectedOption = options.find((option) => option.value === selected);

  return (
    <Popover>
      {({ setIsOpen }) => {
        return (
          <>
            <PopoverTrigger>
              <Button
                aria-label="Select status"
                endIcon={<DropdownArrow />}
                startIcon={selectedOption?.addon}
              >
                {selectedOption ? selectedOption.label : placeholder}
              </Button>
            </PopoverTrigger>
            <PopoverContent>
              <MenuList dense>
                {options.map((option) => (
                  <MenuItem
                    selected={option.value === selected}
                    key={option.value}
                    onClick={() => {
                      setIsOpen(false);
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
            </PopoverContent>
          </>
        );
      }}
    </Popover>
  );
};
