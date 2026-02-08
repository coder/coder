import { Button, buttonVariants } from "components/Button/Button";
import {
	ChevronDownIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
} from "lucide-react";
import { useEffect, useRef } from "react";
import {
	type DayButton,
	DayPicker,
	getDefaultClassNames,
} from "react-day-picker";
import { cn } from "utils/cn";

function Calendar({
	className,
	classNames,
	showOutsideDays = true,
	captionLayout = "label",
	buttonVariant = "subtle",
	locale,
	formatters,
	components,
	...props
}: React.ComponentProps<typeof DayPicker> & {
	buttonVariant?: React.ComponentProps<typeof Button>["variant"];
}) {
	const defaultClassNames = getDefaultClassNames();

	return (
		<DayPicker
			showOutsideDays={showOutsideDays}
			className={cn(
				"p-2 [--cell-size:1.75rem] bg-surface-primary group/calendar",
				className,
			)}
			captionLayout={captionLayout}
			locale={locale}
			formatters={{
				formatMonthDropdown: (date) =>
					date.toLocaleString(undefined, { month: "short" }),
				...formatters,
			}}
			style={
				{
					"--cell-radius": "calc(var(--radius) - 2px)",
				} as React.CSSProperties
			}
			classNames={{
				root: cn("w-fit", defaultClassNames.root),
				months: cn(
					"flex gap-4 flex-col md:flex-row relative",
					defaultClassNames.months,
				),
				month: cn("flex flex-col w-full gap-2", defaultClassNames.month),
				nav: cn(
					"flex items-center gap-1 w-full absolute top-0 inset-x-0 justify-between",
					defaultClassNames.nav,
				),
				button_previous: cn(
					buttonVariants({ variant: buttonVariant }),
					"size-[var(--cell-size)] aria-disabled:opacity-50 p-0 select-none min-w-[var(--cell-size)]",
					defaultClassNames.button_previous,
				),
				button_next: cn(
					buttonVariants({ variant: buttonVariant }),
					"size-[var(--cell-size)] aria-disabled:opacity-50 p-0 select-none min-w-[var(--cell-size)]",
					defaultClassNames.button_next,
				),
				month_caption: cn(
					"flex items-center justify-center h-[var(--cell-size)] w-full px-[var(--cell-size)]",
					defaultClassNames.month_caption,
				),
				dropdowns: cn(
					"w-full flex items-center text-sm font-medium justify-center h-[var(--cell-size)] gap-1.5",
					defaultClassNames.dropdowns,
				),
				dropdown_root: cn(
					"relative cn-calendar-dropdown-root rounded-[var(--cell-radius)]",
					defaultClassNames.dropdown_root,
				),
				dropdown: cn(
					"absolute bg-surface-secondary inset-0 opacity-0",
					defaultClassNames.dropdown,
				),
				caption_label: cn(
					"select-none font-medium",
					captionLayout === "label"
						? "text-sm"
						: "cn-calendar-caption-label rounded-[var(--cell-radius)] flex items-center gap-1 text-sm  [&>svg]:text-content-secondary [&>svg]:size-3.5",
					defaultClassNames.caption_label,
				),
				table: "w-full border-collapse",
				weekdays: cn("flex", defaultClassNames.weekdays),
				weekday: cn(
					"text-content-secondary rounded-[var(--cell-radius)] flex-1 font-normal text-[0.8rem] select-none",
					defaultClassNames.weekday,
				),
				week: cn("flex w-full mt-1", defaultClassNames.week),
				week_number_header: cn(
					"select-none w-[var(--cell-size)]",
					defaultClassNames.week_number_header,
				),
				week_number: cn(
					"text-[0.8rem] select-none text-content-secondary",
					defaultClassNames.week_number,
				),
				day: cn(
					"relative w-full rounded-[var(--cell-radius)] h-full p-0 text-center [&:last-child[data-selected=true]_button]:rounded-r-[var(--cell-radius)] group/day aspect-square select-none",
					props.showWeekNumber
						? "[&:nth-child(2)[data-selected=true]_button]:rounded-l-[var(--cell-radius)]"
						: "[&:first-child[data-selected=true]_button]:rounded-l-[var(--cell-radius)]",
					defaultClassNames.day,
				),
				range_start: cn(
					"rounded-l-[var(--cell-radius)] bg-surface-secondary relative after:content-[''] after:bg-surface-secondary after:absolute after:inset-y-0 after:w-4 after:right-0 z-0 isolate",
					defaultClassNames.range_start,
				),
				range_middle: cn("rounded-none", defaultClassNames.range_middle),
				range_end: cn(
					"rounded-r-[var(--cell-radius)] bg-surface-secondary relative after:content-[''] after:bg-surface-secondary after:absolute after:inset-y-0 after:w-4 after:left-0 z-0 isolate",
					defaultClassNames.range_end,
				),
				outside: cn(
					"text-content-secondary aria-selected:text-content-secondary",
					defaultClassNames.outside,
				),
				disabled: cn(
					"text-content-secondary opacity-50",
					defaultClassNames.disabled,
				),
				hidden: cn("invisible", defaultClassNames.hidden),
				...classNames,
			}}
			components={{
				Root: ({ className, rootRef, ...props }) => {
					return (
						<div
							data-slot="calendar"
							ref={rootRef}
							className={cn(className)}
							{...props}
						/>
					);
				},
				Chevron: ({ className, orientation, ...props }) => {
					if (orientation === "left") {
						return (
							<ChevronLeftIcon
								className={cn("cn-rtl-flip size-4", className)}
								{...props}
							/>
						);
					}

					if (orientation === "right") {
						return (
							<ChevronRightIcon
								className={cn("cn-rtl-flip size-4", className)}
								{...props}
							/>
						);
					}

					return (
						<ChevronDownIcon className={cn("size-4", className)} {...props} />
					);
				},
				DayButton: ({ ...props }) => <CalendarDayButton {...props} />,
				WeekNumber: ({ children, ...props }) => {
					return (
						<td {...props}>
							<div className="flex size-[var(--cell-size)] items-center justify-center text-center">
								{children}
							</div>
						</td>
					);
				},
				...components,
			}}
			{...props}
		/>
	);
}

function CalendarDayButton({
	className,
	day,
	modifiers,
	...props
}: React.ComponentProps<typeof DayButton>) {
	const defaultClassNames = getDefaultClassNames();

	const ref = useRef<HTMLButtonElement>(null);
	useEffect(() => {
		if (modifiers.focused) ref.current?.focus();
	}, [modifiers.focused]);

	return (
		<Button
			ref={ref}
			variant="subtle"
			size="icon"
			data-day={day.date.toLocaleDateString()}
			data-selected-single={
				modifiers.selected &&
				!modifiers.range_start &&
				!modifiers.range_end &&
				!modifiers.range_middle
			}
			data-outside={modifiers.outside}
			data-range-start={modifiers.range_start}
			data-range-end={modifiers.range_end}
			data-range-middle={modifiers.range_middle}
			className={cn(
				"data-[outside=true]:text-content-disabled data-[outside=true]:pointer-events-none",
				"data-[selected-single=true]:bg-surface-primary data-[selected-single=true]:text-content-invert",
				"data-[range-middle=true]:bg-surface-secondary data-[range-middle=true]:text-content-primary",
				"data-[range-start=true]:bg-surface-tertiary data-[range-start=true]:text-content-primary",
				"data-[range-end=true]:bg-surface-tertiary data-[range-end=true]:text-content-primary",
				"group-data-[focused=true]/day:border-ring group-data-[focused=true]/day:ring-content-link",
				"dark:hover:text-content-primary relative isolate z-10 flex aspect-square size-auto w-full",
				"min-w-[var(--cell-size)] flex-col gap-1 border-0 leading-none font-normal",
				"group-data-[focused=true]/day:relative group-data-[focused=true]/day:z-10",
				"group-data-[focused=true]/day:ring-1 data-[range-end=true]:rounded-[var(--cell-radius)]",
				"data-[range-end=true]:rounded-r-[var(--cell-radius)] data-[range-middle=true]:rounded-none",
				"data-[range-start=true]:rounded-[var(--cell-radius)] data-[range-start=true]:rounded-l-[var(--cell-radius)]",
				"[&>span]:text-xs [&>span]:opacity-70",
				defaultClassNames.day,
				className,
			)}
			{...props}
		/>
	);
}

export { Calendar, CalendarDayButton };
