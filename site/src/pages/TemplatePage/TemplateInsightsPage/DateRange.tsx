import "react-date-range/dist/styles.css";
import "react-date-range/dist/theme/default.css";
import type { Interpolation, Theme } from "@emotion/react";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import Button from "@mui/material/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import {
	addDays,
	addHours,
	format,
	isToday,
	startOfDay,
	startOfHour,
	subDays,
} from "date-fns";
import { IDEAL_REFRESH_ONE_MINUTE, useTimeSync } from "hooks/useTimeSync";
import { type ComponentProps, type FC, useRef, useState } from "react";
import { DateRangePicker, createStaticRanges } from "react-date-range";

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

interface DateRangeProps {
	value: DateRangeValue;
	onChange: (value: DateRangeValue) => void;
}

export const DateRange: FC<DateRangeProps> = ({ value, onChange }) => {
	const currentTime = useTimeSync({
		targetRefreshInterval: IDEAL_REFRESH_ONE_MINUTE,
	});
	const selectionStatusRef = useRef<"idle" | "selecting">("idle");
	const [ranges, setRanges] = useState<RangesState>([
		{
			...value,
			key: "selection",
		},
	]);
	const [open, setOpen] = useState(false);

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger>
				<Button>
					<span>{format(value.startDate, "MMM d, Y")}</span>
					<ArrowRightAltOutlined
						css={{ width: 16, height: 16, marginLeft: 8, marginRight: 8 }}
					/>
					<span>{format(value.endDate, "MMM d, Y")}</span>
				</Button>
			</PopoverTrigger>
			<PopoverContent>
				<DateRangePicker
					css={styles.wrapper}
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
						const now = new Date();
						onChange({
							startDate: startOfDay(startDate),
							endDate: isToday(endDate)
								? startOfHour(addHours(now, 1))
								: startOfDay(addDays(endDate, 1)),
						});
						setOpen(false);
					}}
					moveRangeOnFirstSelection={false}
					months={2}
					ranges={ranges}
					maxDate={currentTime}
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
			</PopoverContent>
		</Popover>
	);
};

const styles = {
	wrapper: (theme) => ({
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
				background: theme.palette.background.paper,
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
			backgroundColor: theme.palette.background.paper,
		},

		"& .rdrDateDisplayItem": {
			background: "transparent",
			borderColor: theme.palette.divider,

			"& input": {
				color: theme.palette.text.secondary,
			},

			"&.rdrDateDisplayItemActive": {
				borderColor: theme.palette.text.primary,
				backgroundColor: theme.palette.background.paper,

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
	}),
} satisfies Record<string, Interpolation<Theme>>;
