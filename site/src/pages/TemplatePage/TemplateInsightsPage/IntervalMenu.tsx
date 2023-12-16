import CheckOutlined from "@mui/icons-material/CheckOutlined";
import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Button from "@mui/material/Button";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { type FC, useState, useRef } from "react";

export const insightsIntervals = {
  day: {
    label: "Daily",
  },
  week: {
    label: "Weekly",
  },
} as const;

export type InsightsInterval = keyof typeof insightsIntervals;

interface IntervalMenuProps {
  value: InsightsInterval;
  onChange: (value: InsightsInterval) => void;
}

export const IntervalMenu: FC<IntervalMenuProps> = ({ value, onChange }) => {
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
        {insightsIntervals[value].label}
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
        {Object.entries(insightsIntervals).map(([interval, { label }]) => {
          return (
            <MenuItem
              css={{ fontSize: 14, justifyContent: "space-between" }}
              key={interval}
              onClick={() => {
                onChange(interval as InsightsInterval);
                handleClose();
              }}
            >
              {label}
              <div css={{ width: 16, height: 16 }}>
                {value === interval && (
                  <CheckOutlined css={{ width: 16, height: 16 }} />
                )}
              </div>
            </MenuItem>
          );
        })}
      </Menu>
    </div>
  );
};
