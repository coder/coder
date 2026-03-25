/**
 * Adapted from shadcn/ui on 2025-03-24.
 * @see {@link https://ui.shadcn.com/docs/components/calendar}
 *
 * Built on top of React DayPicker v9. Styled with Tailwind using the
 * project's existing design tokens so it matches every other primitive
 * in the component library.
 */

import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";
import type { ComponentProps } from "react";
import {
	type DayButton,
	DayPicker,
	getDefaultClassNames,
} from "react-day-picker";
import { cn } from "utils/cn";
import { Button, type ButtonProps } from "#/components/Button/Button";

function Calendar({
	className,
	classNames,
	showOutsideDays = true,
	captionLayout = "label",
	buttonVariant = "subtle",
	formatters,
	components,
	...props
}: ComponentProps<typeof DayPicker> & {
	buttonVariant?: ButtonProps["variant"];
}) {
	const defaultClassNames = getDefaultClassNames();

	return (
		<DayPicker
			showOutsideDays={showOutsideDays}
			className={cn(
				"bg-surface-primary group/calendar p-3 [--cell-size:2rem]",
				className,
			)}
			captionLayout={captionLayout}
			formatters={{
				formatMonthDropdown: (date) =>
					date.toLocaleString("default", { month: "short" }),
				...formatters,
			}}
			classNames={{
				root: cn("w-fit", defaultClassNames.root),
				months: cn(
					"relative flex flex-col gap-4 md:flex-row",
					defaultClassNames.months,
				),
				month: cn("flex w-full flex-col gap-4", defaultClassNames.month),
				nav: cn(
					"absolute inset-x-0 top-0 flex w-full items-center justify-between gap-1",
					defaultClassNames.nav,
				),
				button_previous: cn(
					"h-[--cell-size] w-[--cell-size] select-none p-0",
					"inline-flex items-center justify-center rounded-md",
					"bg-transparent border-0 cursor-pointer",
					"text-content-secondary hover:text-content-primary hover:bg-surface-secondary",
					"aria-disabled:opacity-50",
					defaultClassNames.button_previous,
				),
				button_next: cn(
					"h-[--cell-size] w-[--cell-size] select-none p-0",
					"inline-flex items-center justify-center rounded-md",
					"bg-transparent border-0 cursor-pointer",
					"text-content-secondary hover:text-content-primary hover:bg-surface-secondary",
					"aria-disabled:opacity-50",
					defaultClassNames.button_next,
				),
				month_caption: cn(
					"flex h-[--cell-size] w-full items-center justify-center px-[--cell-size]",
					defaultClassNames.month_caption,
				),
				dropdowns: cn(
					"flex h-[--cell-size] w-full items-center justify-center gap-1.5 text-sm font-medium",
					defaultClassNames.dropdowns,
				),
				dropdown_root: cn(
					"has-focus:border-content-link border-border-default relative rounded-md border",
					defaultClassNames.dropdown_root,
				),
				dropdown: cn(
					"bg-surface-primary absolute inset-0 opacity-0",
					defaultClassNames.dropdown,
				),
				caption_label: cn(
					"select-none font-medium text-sm text-content-primary",
					defaultClassNames.caption_label,
				),
				table:
					"w-full border-collapse border-0 [&_td]:border-0 [&_th]:border-0",
				weekdays: cn("flex", defaultClassNames.weekdays),
				weekday: cn(
					"text-content-secondary flex-1 select-none rounded-md text-[0.8rem] font-normal",
					defaultClassNames.weekday,
				),
				week: cn("mt-2 flex w-full", defaultClassNames.week),
				week_number_header: cn(
					"w-[--cell-size] select-none",
					defaultClassNames.week_number_header,
				),
				week_number: cn(
					"text-content-secondary select-none text-[0.8rem]",
					defaultClassNames.week_number,
				),
				day: cn(
					"group/day relative aspect-square h-full w-full select-none p-0 text-center",
					"[&:first-child[data-selected=true]_button]:rounded-l-md",
					"[&:last-child[data-selected=true]_button]:rounded-r-md",
					defaultClassNames.day,
				),
				range_start: cn(
					"bg-surface-tertiary rounded-l-md",
					defaultClassNames.range_start,
				),
				range_middle: cn("rounded-none", defaultClassNames.range_middle),
				range_end: cn(
					"bg-surface-tertiary rounded-r-md",
					defaultClassNames.range_end,
				),
				today: cn(
					"bg-surface-tertiary text-content-primary rounded-md data-[selected=true]:rounded-none",
					defaultClassNames.today,
				),
				outside: cn(
					"text-content-disabled aria-selected:text-content-disabled",
					defaultClassNames.outside,
				),
				disabled: cn(
					"text-content-disabled opacity-50",
					defaultClassNames.disabled,
				),
				hidden: cn("invisible", defaultClassNames.hidden),
				...classNames,
			}}
			components={{
				Chevron: ({
					className,
					orientation,
					size: _size,
					disabled: _disabled,
					...rest
				}) => {
					const Icon =
						orientation === "left" ? ChevronLeftIcon : ChevronRightIcon;
					return <Icon className={cn("size-4", className)} {...rest} />;
				},
				DayButton: CalendarDayButton,
				WeekNumber: ({ children, ...weekProps }) => (
					<td {...weekProps}>
						<div className="flex size-[--cell-size] items-center justify-center text-center">
							{children}
						</div>
					</td>
				),
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
}: ComponentProps<typeof DayButton>) {
	const defaultClassNames = getDefaultClassNames();

	return (
		<Button
			variant="subtle"
			size="icon"
			data-day={day.date.toLocaleDateString()}
			data-selected-single={
				modifiers.selected &&
				!modifiers.range_start &&
				!modifiers.range_end &&
				!modifiers.range_middle
			}
			data-range-start={modifiers.range_start}
			data-range-end={modifiers.range_end}
			data-range-middle={modifiers.range_middle}
			className={cn(
				"flex aspect-square h-auto w-full min-w-[--cell-size] flex-col gap-1 font-normal leading-none",
				"data-[selected-single=true]:bg-surface-invert-primary data-[selected-single=true]:text-content-invert",
				"data-[range-middle=true]:bg-surface-tertiary data-[range-middle=true]:text-content-primary",
				"data-[range-start=true]:bg-surface-invert-primary data-[range-start=true]:text-content-invert",
				"data-[range-end=true]:bg-surface-invert-primary data-[range-end=true]:text-content-invert",
				"data-[range-end=true]:rounded-md data-[range-middle=true]:rounded-none data-[range-start=true]:rounded-md",
				"group-data-[focused=true]/day:relative group-data-[focused=true]/day:z-10",
				"group-data-[focused=true]/day:ring-2 group-data-[focused=true]/day:ring-content-link",
				defaultClassNames.day,
				className,
			)}
			{...props}
		/>
	);
}

export { Calendar };
