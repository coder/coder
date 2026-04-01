/**
 * Copied from shadc/ui on 12/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/breadcrumb}
 */
import { Slot } from "@radix-ui/react-slot";
import { MoreHorizontal } from "lucide-react";
import { cn } from "utils/cn";

type BreadcrumbProps = React.ComponentPropsWithRef<"nav"> & {
	separator?: React.ReactNode;
};

export const Breadcrumb: React.FC<BreadcrumbProps> = ({ ...props }) => {
	return <nav aria-label="breadcrumb" {...props} />;
};

export const BreadcrumbList: React.FC<React.ComponentPropsWithRef<"ol">> = ({
	className,
	...props
}) => {
	return (
		<ol
			className={cn(
				"flex flex-wrap items-center text-sm pl-6 my-4 gap-1.5 break-words font-medium list-none sm:gap-2.5",
				className,
			)}
			{...props}
		/>
	);
};

export const BreadcrumbItem: React.FC<React.ComponentPropsWithRef<"li">> = ({
	className,
	...props
}) => {
	return (
		<li
			className={cn(
				"inline-flex items-center gap-1.5 text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};

type BreadcrumbLinkProps = React.ComponentPropsWithRef<"a"> & {
	asChild?: boolean;
};

export const BreadcrumbLink: React.FC<BreadcrumbLinkProps> = ({
	asChild,
	className,
	...props
}) => {
	const Comp = asChild ? Slot : "a";

	return (
		<Comp
			className={cn(
				"text-content-secondary transition-colors hover:text-content-primary no-underline hover:underline",
				className,
			)}
			{...props}
		/>
	);
};

export const BreadcrumbPage: React.FC<React.ComponentPropsWithRef<"span">> = ({
	className,
	...props
}) => {
	return (
		<span
			aria-current="page"
			className={cn(
				"flex items-center gap-2 text-content-secondary",
				className,
			)}
			{...props}
		/>
	);
};

export const BreadcrumbSeparator: React.FC<
	Omit<React.ComponentPropsWithRef<"li">, "children">
> = ({ className, ...props }) => {
	return (
		<li
			role="presentation"
			aria-hidden="true"
			className={cn(
				"text-content-disabled [&>svg]:w-3.5 [&>svg]:h-3.5",
				className,
			)}
			{...props}
		>
			/
		</li>
	);
};

export const BreadcrumbEllipsis: React.FC<
	Omit<React.ComponentPropsWithRef<"span">, "children">
> = ({ className, ...props }) => {
	return (
		<span
			role="presentation"
			aria-hidden="true"
			className={cn("flex h-9 w-9 items-center justify-center", className)}
			{...props}
		>
			<MoreHorizontal className="h-4 w-4" />
			<span className="sr-only">More</span>
		</span>
	);
};
