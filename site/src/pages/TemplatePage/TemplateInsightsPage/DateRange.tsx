import Box from "@mui/material/Box";
import { styled } from "@mui/material/styles";
import { ComponentProps, useRef, useState } from "react";
import "react-date-range/dist/styles.css";
import "react-date-range/dist/theme/default.css";
import Button from "@mui/material/Button";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import Popover from "@mui/material/Popover";
import { DateRangePicker, createStaticRanges } from "react-date-range";
import {
  addDays,
  addHours,
  format,
  isToday,
  startOfDay,
  startOfHour,
  subDays,
} from "date-fns";

// The type definition from @types is wrong
declare module "react-date-range" {
  export function createStaticRanges(
    ranges: Omit<StaticRange, "isSelected">[],
  ): StaticRange[];
}

export type DateRangeValue = {
  startDate: Date;
  endDate: Date;
};

type RangesState = NonNullable<
  ComponentProps<typeof DateRangePicker>["ranges"]
>;

export const DateRange = ({
  value,
  onChange,
}: {
  value: DateRangeValue;
  onChange: (value: DateRangeValue) => void;
}) => {
  const selectionStatusRef = useRef<"idle" | "selecting">("idle");
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [ranges, setRanges] = useState<RangesState>([
    {
      ...value,
      key: "selection",
    },
  ]);
  const currentRange = {
    startDate: ranges[0].startDate as Date,
    endDate: ranges[0].endDate as Date,
  };
  const handleClose = () => {
    const now = new Date();
    onChange({
      startDate: startOfDay(currentRange.startDate),
      endDate: isToday(currentRange.endDate)
        ? startOfHour(addHours(now, 1))
        : startOfDay(addDays(currentRange.endDate, 1)),
    });
    setIsOpen(false);
  };

  return (
    <>
      <Button ref={anchorRef} onClick={() => setIsOpen(true)}>
        <span>{format(currentRange.startDate, "MMM d, Y")}</span>
        <ArrowRightAltOutlined sx={{ width: 16, height: 16, mx: 1 }} />
        <span>{format(currentRange.endDate, "MMM d, Y")}</span>
      </Button>
      <Popover
        anchorEl={anchorRef.current}
        open={isOpen}
        onClose={handleClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        sx={{
          "& .MuiPaper-root": {
            marginTop: 1,
          },
        }}
      >
        <DateRangePickerWrapper
          component={DateRangePicker}
          onChange={(item) => {
            const range = item.selection;
            setRanges([range]);

            // When it is the first selection, we don't want to close the popover
            // We have to do that ourselves because the library doesn't provide a way to do it
            if (selectionStatusRef.current === "idle") {
              selectionStatusRef.current = "selecting";
              return;
            }

            selectionStatusRef.current = "idle";
            const startDate = range.startDate as Date;
            const endDate = range.endDate as Date;
            onChange({
              startDate,
              endDate,
            });
            setIsOpen(false);
          }}
          moveRangeOnFirstSelection={false}
          months={2}
          ranges={ranges}
          maxDate={new Date()}
          direction="horizontal"
          staticRanges={createStaticRanges([
            {
              label: "Today",
              range: () => ({
                startDate: new Date(),
                endDate: new Date(),
              }),
            },
            {
              label: "Yesterday",
              range: () => ({
                startDate: subDays(new Date(), 1),
                endDate: subDays(new Date(), 1),
              }),
            },
            {
              label: "Last 7 days",
              range: () => ({
                startDate: subDays(new Date(), 6),
                endDate: new Date(),
              }),
            },
            {
              label: "Last 14 days",
              range: () => ({
                startDate: subDays(new Date(), 13),
                endDate: new Date(),
              }),
            },
            {
              label: "Last 30 days",
              range: () => ({
                startDate: subDays(new Date(), 29),
                endDate: new Date(),
              }),
            },
          ])}
        />
      </Popover>
    </>
  );
};

const DateRangePickerWrapper: typeof Box = styled(Box)(({ theme }) => ({
  "& .rdrDefinedRangesWrapper": {
    background: theme.palette.background.paper,
    borderColor: theme.palette.divider,
  },

  "& .rdrStaticRange": {
    background: theme.palette.background.paper,
    border: 0,
    fontSize: 14,
    color: theme.palette.text.secondary,

    "&:hover .rdrStaticRangeLabel": {
      background: theme.palette.background.paperLight,
      color: theme.palette.text.primary,
    },

    "&.rdrStaticRangeSelected": {
      color: `${theme.palette.text.primary} !important`,
    },
  },

  "& .rdrInputRanges": {
    display: "none",
  },

  "& .rdrDateDisplayWrapper": {
    backgroundColor: theme.palette.background.paper,
  },

  "& .rdrCalendarWrapper": {
    backgroundColor: theme.palette.background.paperLight,
  },

  "& .rdrDateDisplayItem": {
    background: "transparent",
    borderColor: theme.palette.divider,

    "& input": {
      color: theme.palette.text.secondary,
    },

    "&.rdrDateDisplayItemActive": {
      borderColor: theme.palette.text.primary,
      backgroundColor: theme.palette.background.paperLight,

      "& input": {
        color: theme.palette.text.primary,
      },
    },
  },

  "& .rdrMonthPicker select, & .rdrYearPicker select": {
    color: theme.palette.text.primary,
    appearance: "auto",
    background: "transparent",
  },

  "& .rdrMonthName, & .rdrWeekDay": {
    color: theme.palette.text.secondary,
  },

  "& .rdrDayPassive .rdrDayNumber span": {
    color: theme.palette.text.disabled,
  },

  "& .rdrDayNumber span": {
    color: theme.palette.text.primary,
  },

  "& .rdrDayToday .rdrDayNumber span": {
    fontWeight: 900,

    "&:after": {
      display: "none",
    },
  },

  "& .rdrInRange, & .rdrEndEdge, & .rdrStartEdge": {
    color: theme.palette.primary.main,
  },

  "& .rdrDayDisabled": {
    backgroundColor: "transparent",

    "& .rdrDayNumber span": {
      color: theme.palette.text.disabled,
    },
  },
}));
