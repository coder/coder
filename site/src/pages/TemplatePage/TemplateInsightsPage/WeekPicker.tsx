import CheckOutlined from "@mui/icons-material/CheckOutlined";
import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { useState, useRef } from "react";
import { DateRangeValue } from "./DateRange";
import { differenceInWeeks } from "date-fns";
import { lastWeeks } from "./utils";

export const numberOfWeeksOptions = [4, 12, 24, 48] as const;

export const WeekPicker = ({
  value,
  onChange,
}: {
  value: DateRangeValue;
  onChange: (value: DateRangeValue) => void;
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  // Why +1? If you get the week 1 and week 2 the diff is 1, but there are 2 weeks
  const numberOfWeeks = differenceInWeeks(value.endDate, value.startDate) + 1;

  const handleClose = () => {
    setOpen(false);
  };

  return (
    <div>
      <Button
        ref={anchorRef}
        id="interval-button"
        aria-controls={open ? "interval-menu" : undefined}
        aria-haspopup="true"
        aria-expanded={open ? "true" : undefined}
        onClick={() => setOpen(true)}
        endIcon={<ExpandMoreOutlined />}
      >
        Last {numberOfWeeks} weeks
      </Button>
      <Menu
        id="interval-menu"
        anchorEl={anchorRef.current}
        open={open}
        onClose={handleClose}
        MenuListProps={{
          "aria-labelledby": "interval-button",
        }}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
      >
        {numberOfWeeksOptions.map((option) => {
          const optionRange = lastWeeks(option);

          return (
            <MenuItem
              css={{ fontSize: 14, justifyContent: "space-between" }}
              key={option}
              onClick={() => {
                onChange(optionRange);
                handleClose();
              }}
            >
              Last {option} weeks
              <Box css={{ width: 16, height: 16 }}>
                {numberOfWeeks === option && (
                  <CheckOutlined css={{ width: 16, height: 16 }} />
                )}
              </Box>
            </MenuItem>
          );
        })}
      </Menu>
    </div>
  );
};
