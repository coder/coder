/**
 * Copied from shadc/ui on 02/03/2025
 * @see {@link https://ui.shadcn.com/docs/components/table}
 */

import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";
import { cn } from "utils/cn";

export const Table = React.forwardRef<
	HTMLTableElement,
	React.HTMLAttributes<HTMLTableElement>
>(({ className, ...props }, ref) => (
	<div className="relative w-full overflow-auto">
		<table
			ref={ref}
			className={cn(
				"w-full caption-bottom text-xs font-medium text-content-secondary border-separate border-spacing-0",
				className,
			)}
			{...props}
		/>
	</div>
));

export const TableHeader = React.forwardRef<
	HTMLTableSectionElement,
	React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
	<thead ref={ref} className={cn("[&_td]:border-none", className)} {...props} />
));

export const TableBody = React.forwardRef<
	HTMLTableSectionElement,
	React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
	<tbody
		ref={ref}
		className={cn(
			"[&>tr:first-of-type>td]:border-t [&>tr>td:first-of-type]:border-l",
			"[&>tr:last-child>td]:border-b [&>tr>td:last-child]:border-r",
			"[&>tr:first-of-type>td:first-of-type]:rounded-tl-md [&>tr:first-of-type>td:last-child]:rounded-tr-md",
			"[&>tr:last-child>td:first-of-type]:rounded-bl-md [&>tr:last-child>td:last-child]:rounded-br-md",
			className,
		)}
		{...props}
	/>
));

export const TableFooter = React.forwardRef<
	HTMLTableSectionElement,
	React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
	<tfoot
		ref={ref}
		className={cn(
			"border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
			className,
		)}
		{...props}
	/>
));

const tableRowVariants = cva(
	[
		"border-0 border-b border-solid border-border transition-colors",
		"data-[state=selected]:bg-muted",
	],
	{
		variants: {
			hover: {
				false: null,
				true: cn([
					"cursor-pointer hover:outline focus:outline outline-1 -outline-offset-1 outline-border-hover",
					"first:rounded-t-md last:rounded-b-md",
				]),
			},
		},
		defaultVariants: {
			hover: false,
		},
	},
);

export type TableRowProps = React.HTMLAttributes<HTMLTableRowElement> &
	VariantProps<typeof tableRowVariants>;

export const TableRow = React.forwardRef<HTMLTableRowElement, TableRowProps>(
	({ className, hover, ...props }, ref) => (
		<tr
			ref={ref}
			className={cn(
				"border-0 border-b border-solid border-border transition-colors",
				"data-[state=selected]:bg-muted",
				tableRowVariants({ hover }),
				className,
			)}
			{...props}
		/>
	),
);

export const TableHead = React.forwardRef<
	HTMLTableCellElement,
	React.ThHTMLAttributes<HTMLTableCellElement>
>(({ className, ...props }, ref) => (
	<th
		ref={ref}
		className={cn(
			"p-3 text-left align-middle font-semibold",
			"[&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
			className,
		)}
		{...props}
	/>
));

export const TableCell = React.forwardRef<
	HTMLTableCellElement,
	React.TdHTMLAttributes<HTMLTableCellElement>
>(({ className, ...props }, ref) => (
	<td
		ref={ref}
		className={cn(
			"border-0 border-t border-border border-solid",
			"p-3 align-middle [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
			className,
		)}
		{...props}
	/>
));

const _TableCaption = React.forwardRef<
	HTMLTableCaptionElement,
	React.HTMLAttributes<HTMLTableCaptionElement>
>(({ className, ...props }, ref) => (
	<caption
		ref={ref}
		className={cn("mt-4 text-sm text-muted-foreground", className)}
		{...props}
	/>
));
