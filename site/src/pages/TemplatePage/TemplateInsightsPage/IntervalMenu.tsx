import CheckOutlined from "@mui/icons-material/CheckOutlined";
import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { useState, MouseEvent } from "react";

export const insightsIntervals = {
  day: {
    label: "Daily",
  },
  week: {
    label: "Weekly",
  },
} as const;

export type InsightsInterval = keyof typeof insightsIntervals;

export const IntervalMenu = ({
  value,
  onChange,
}: {
  value: InsightsInterval;
  onChange: (value: InsightsInterval) => void;
}) => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };
  const handleClose = () => {
    setAnchorEl(null);
  };

  return (
    <div>
      <Button
        id="interval-button"
        aria-controls={open ? "interval-menu" : undefined}
        aria-haspopup="true"
        aria-expanded={open ? "true" : undefined}
        onClick={handleClick}
        endIcon={<ExpandMoreOutlined />}
      >
        {insightsIntervals[value].label}
      </Button>
      <Menu
        id="interval-menu"
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
        MenuListProps={{
          "aria-labelledby": "interval-button",
        }}
      >
        {Object.keys(insightsIntervals).map((interval) => {
          const { label } = insightsIntervals[interval as InsightsInterval];
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
              <Box css={{ width: 16, height: 16 }}>
                {value === interval && (
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
