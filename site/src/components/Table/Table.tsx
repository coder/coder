/**
 * Copied from shadc/ui on 02/03/2025
 * @see {@link https://ui.shadcn.com/docs/components/table}
 */
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "utils/cn";

export const Table: React.FC<React.ComponentPropsWithRef<"table">> = ({
	className,
	...props
}) => {
	return (
		<div className="relative w-full overflow-auto">
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

export const TableBody: React.FC<React.ComponentPropsWithRef<"tbody">> = ({
	className,
	...props
}) => {
	return (
		<tbody
			className={cn(
				"[&>tr:first-of-type>td]:border-t [&>tr>td:first-of-type]:border-l",
				"[&>tr:last-child>td]:border-b [&>tr>td:last-child]:border-r",
				"[&>tr:first-of-type>td:first-of-type]:rounded-tl-md [&>tr:first-of-type>td:last-child]:rounded-tr-md",
				"[&>tr:last-child>td:first-of-type]:rounded-bl-md [&>tr:last-child>td:last-child]:rounded-br-md",
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
				"border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
				className,
			)}
			{...props}
		/>
	);
};

const tableRowVariants = cva(
	[
		"border-0 border-b border-solid border-border transition-colors",
		"data-[state=selected]:bg-muted",
	],
	{
		variants: {
			hover: {
				false: null,
				true: cn(
					"cursor-pointer hover:outline focus:outline outline-1 -outline-offset-1 outline-border-hover",
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
				"border-0 border-b border-solid border-border transition-colors",
				"data-[state=selected]:bg-muted",
				tableRowVariants({ hover }),
				className,
			)}
			{...props}
		/>
	);
};

export const TableHead: React.FC<React.ComponentPropsWithRef<"th">> = ({
	className,
	...props
}) => {
	return (
		<th
			className={cn(
				"p-3 text-left align-middle font-semibold",
				"[&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
				className,
			)}
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
			className={cn(
				"border-0 border-t border-border border-solid",
				"p-3 align-middle [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
				className,
			)}
			{...props}
		/>
	);
};
