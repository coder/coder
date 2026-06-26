import { type FC, type ReactNode, useId } from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";

type FormFieldProps = React.ComponentPropsWithRef<"input"> & {
	field: FormHelpers;
	label: ReactNode;
	description?: ReactNode;
};

export const FormField: FC<FormFieldProps> = ({
	field,
	label,
	description,
	className,
	...inputProps
}) => {
	const generatedId = useId();
	const id = inputProps.id ?? generatedId;
	const errorId = `${id}-error`;
	const helperId = `${id}-helper`;
	const descriptionId = `${id}-description`;
	const describedBy = [
		description ? descriptionId : null,
		field.error ? errorId : field.helperText ? helperId : null,
	]
		.filter(Boolean)
		.join(" ");
	const required = inputProps.required ?? false;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={id}>
				{label}
				{required && (
					<>
						{" "}
						<span className="text-xs font-bold text-content-destructive">
							*
						</span>
					</>
				)}
			</Label>
			{description && (
				<div id={descriptionId} className="text-xs text-content-secondary">
					{description}
				</div>
			)}
			<Input
				name={field.name}
				value={field.value}
				onChange={field.onChange}
				onBlur={field.onBlur}
				{...inputProps}
				id={id}
				aria-invalid={field.error}
				aria-describedby={describedBy || undefined}
				className={cn(field.error && "border-border-destructive", className)}
			/>
			{field.error ? (
				<span id={errorId} className="text-xs text-content-destructive">
					{field.helperText}
				</span>
			) : (
				field.helperText && (
					<span id={helperId} className="text-xs text-content-secondary">
						{field.helperText}
					</span>
				)
			)}
		</div>
	);
};
