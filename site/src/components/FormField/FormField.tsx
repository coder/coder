import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { type FC, type ReactNode, useId } from "react";
import { cn } from "utils/cn";
import type { FormHelpers } from "utils/formUtils";

type FormFieldProps = React.ComponentPropsWithRef<"input"> & {
	field: FormHelpers;
	label: ReactNode;
};

export const FormField: FC<FormFieldProps> = ({
	field,
	label,
	className,
	...inputProps
}) => {
	const generatedId = useId();
	const id = inputProps.id ?? generatedId;
	const errorId = `${id}-error`;
	const helperId = `${id}-helper`;

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={id}>{label}</Label>
			<Input
				{...inputProps}
				id={id}
				aria-invalid={field.error}
				aria-describedby={
					field.error ? errorId : field.helperText ? helperId : undefined
				}
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
