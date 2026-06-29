/**
 * Copied from shadc/ui on 02/03/2025
 * @see {@link https://ui.shadcn.com/docs/components/table}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "#/utils/cn";

type TableProps = React.ComponentPropsWithRef<"table"> & {
	wrapperClassName?: string;
};

export const Table: React.FC<TableProps> = ({
	className,
	wrapperClassName,
	...props
}) => {
	return (
		<div className={cn("relative w-full overflow-auto", wrapperClassName)}>
			<table
				className={cn(
					"w-full caption-bottom text-xs font-medium text-content-secondary border-separate border-spacing-0",
					className,
				)}
				{...props}
			/>
		</div>
	);
};

export const TableHeader: React.FC<React.ComponentPropsWithRef<"thead">> = ({
	className,
	...props
}) => {
	return <thead className={cn("[&_td]:border-none", className)} {...props} />;
};

const tableBodyVariants = cva(null, {
	variants: {
		size: {
			lg: "[&>tr>td]:box-border [&>tr>td]:h-[72px]",
		},
	},
});

type TableBodyProps = React.ComponentPropsWithRef<"tbody"> &
	VariantProps<typeof tableBodyVariants>;

export const TableBody: React.FC<TableBodyProps> = ({
	className,
	size,
	...props
}) => {
	return (
		<tbody
			className={cn(
				"[&>tr:first-of-type>td]:border-t [&>tr>td:first-of-type]:border-l",
				"[&>tr:last-child>td]:border-b [&>tr>td:last-child]:border-r",
				"[&>tr:first-of-type>td:first-of-type]:rounded-tl-md [&>tr:first-of-type>td:last-child]:rounded-tr-md",
				"[&>tr:last-child>td:first-of-type]:rounded-bl-md [&>tr:last-child>td:last-child]:rounded-br-md",
				tableBodyVariants({ size }),
				className,
			)}
			{...props}
		/>
	);
};

export const TableFooter: React.FC<React.ComponentPropsWithRef<"tfoot">> = ({
	className,
	...props
}) => {
	return (
		<tfoot
			className={cn(
				"border-t bg-surface-secondary/50 font-medium [&>tr]:last:border-b-0",
				className,
			)}
			{...props}
		/>
	);
};

const tableRowVariants = cva(
	[
		"border-0 border-b border-solid border-border transition-colors",
		"data-[state=selected]:bg-surface-secondary",
	],
	{
		variants: {
			hover: {
				false: null,
				true: cn(
					"cursor-pointer hover:outline focus-visible:outline outline-1 -outline-offset-1 outline-border-secondary",
					"first:rounded-t-md last:rounded-b-md",
				),
			},
		},
		defaultVariants: {
			hover: false,
		},
	},
);

export type TableRowProps = React.HTMLAttributes<HTMLTableRowElement> &
	VariantProps<typeof tableRowVariants>;

export const TableRow: React.FC<TableRowProps> = ({
	className,
	hover,
	...props
}) => {
	return (
		<tr
			className={cn(
				tableRowVariants({ hover }),
				"data-[state=selected]:bg-surface-secondary",
				className,
			)}
			{...props}
		/>
	);
};

export const TableHead: React.FC<React.ComponentPropsWithRef<"th">> = ({
	className,
	scope = "col",
	...props
}) => {
	return (
		<th
			className={cn(
				"p-3 text-left align-middle font-semibold",
				"[&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
				className,
			)}
			scope={scope}
			{...props}
		/>
	);
};

export const TableCell: React.FC<React.ComponentPropsWithRef<"td">> = ({
	className,
	...props
}) => {
	return (
		<td
			{...props}
			className={cn(
				"border-0 border-t border-border border-solid",
				"p-3 align-middle [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
				className,
			)}
		/>
	);
};
