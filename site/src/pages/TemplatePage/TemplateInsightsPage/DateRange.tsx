import "react-date-range/dist/styles.css";
import "react-date-range/dist/theme/default.css";
import type { Interpolation, Theme } from "@emotion/react";
import { Button } from "components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import dayjs from "dayjs";
import { MoveRightIcon } from "lucide-react";
import { type ComponentProps, type FC, useRef, useState } from "react";
import { createStaticRanges, DateRangePicker } from "react-date-range";

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
			<PopoverTrigger asChild>
				<Button variant="outline">
					<span>{dayjs(value.startDate).format("MMM D, YYYY")}</span>
					<MoveRightIcon />
					<span>{dayjs(value.endDate).format("MMM D, YYYY")}</span>
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
						const startDate = range!.startDate as Date;
						const endDate = range!.endDate as Date;
						const now = new Date();
						onChange({
							startDate: dayjs(startDate).startOf("day").toDate(),
							endDate: dayjs(endDate).isSame(dayjs(), "day")
								? dayjs(now).startOf("hour").add(1, "hour").toDate()
								: dayjs(endDate).startOf("day").add(1, "day").toDate(),
						});
						setOpen(false);
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
								startDate: dayjs().subtract(1, "day").toDate(),
								endDate: dayjs().subtract(1, "day").toDate(),
							}),
						},
						{
							label: "Last 7 days",
							range: () => ({
								startDate: dayjs().subtract(6, "day").toDate(),
								endDate: new Date(),
							}),
						},
						{
							label: "Last 14 days",
							range: () => ({
								startDate: dayjs().subtract(13, "day").toDate(),
								endDate: new Date(),
							}),
						},
						{
							label: "Last 30 days",
							range: () => ({
								startDate: dayjs().subtract(29, "day").toDate(),
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

			"&:is(:hover, :focus) .rdrStaticRangeLabel": {
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
