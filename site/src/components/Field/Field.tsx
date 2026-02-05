import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "utils/cn";

export function FieldSet({
	className,
	...props
}: React.ComponentProps<"fieldset">) {
	return (
		<fieldset className={cn("flex flex-col gap-4", className)} {...props} />
	);
}

export function FieldLegend({
	className,
	...props
}: React.ComponentProps<"legend">) {
	return <legend className={cn("font-medium mb-1.5", className)} {...props} />;
}

export function FieldGroup({
	className,
	...props
}: React.ComponentProps<"div">) {
	return (
		<div className={cn("flex flex-col gap-5 w-full", className)} {...props} />
	);
}

const fieldVariants = cva(
	"flex gap-1 w-full data-[invalid=true]:text-destructive",
	{
		variants: {
			orientation: {
				vertical: "flex-col *:w-full [&>.sr-only]:w-auto",
				horizontal: "flex-row items-center",
			},
		},
	},
);

export function Field({
	className,
	orientation = "vertical",
	...props
}: React.ComponentProps<"div"> & VariantProps<typeof fieldVariants>) {
	return (
		<div className={cn(fieldVariants({ orientation }), className)} {...props} />
	);
}

export function FieldContent({
	className,
	...props
}: React.ComponentProps<"div">) {
	return (
		<div
			className={cn("flex flex-1 flex-col leading-snug gap-0.5", className)}
			{...props}
		/>
	);
}

export function FieldLabel({
	className,
	...props
}: React.ComponentProps<"label">) {
	return (
		// biome-ignore lint/a11y/noLabelWithoutControl: This is a generic component.
		<label
			className={cn("flex text-sm font-medium w-fit leading-snug", className)}
			{...props}
		/>
	);
}

export function FieldTitle({ ...props }: React.ComponentProps<"div">) {
	return (
		<div
			className="flex items-center leading-snug w-fit gap-2 text-sm font-medium"
			{...props}
		/>
	);
}

export function FieldDescription({
	className,
	...props
}: React.ComponentProps<"div">) {
	return (
		<div
			className={cn("text-content-secondary text-left text-xs mt-1", className)}
			{...props}
		/>
	);
}

export function FieldError({ ...props }: React.ComponentProps<"div">) {
	if (!props.children) return null;
	return (
		<div
			className="text-content-destructive text-left text-xs mt-1"
			{...props}
		/>
	);
}
