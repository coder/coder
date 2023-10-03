import CheckOutlined from "@mui/icons-material/CheckOutlined";
import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { useState, useRef } from "react";

export const weeklyPresets = [4, 12, 24, 48] as const;

export type WeeklyPreset = (typeof weeklyPresets)[number];

export const WeeklyPresetsMenu = ({
  value,
  onChange,
}: {
  value: WeeklyPreset;
  onChange: (value: WeeklyPreset) => void;
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);

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
        Last {value} weeks
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
        {weeklyPresets.map((numberOfWeeks) => {
          return (
            <MenuItem
              css={{ fontSize: 14, justifyContent: "space-between" }}
              key={numberOfWeeks}
              onClick={() => {
                onChange(numberOfWeeks);
                handleClose();
              }}
            >
              Last {numberOfWeeks} weeks
              <Box css={{ width: 16, height: 16 }}>
                {value === numberOfWeeks && (
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
