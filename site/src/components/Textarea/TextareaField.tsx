import type { ReactNode } from "react";
import { cn } from "utils/cn";
import { Textarea } from "./Textarea";

interface TextareaFieldProps extends React.ComponentPropsWithRef<"textarea"> {
	label?: string;
	error?: boolean;
	helperText?: ReactNode;
	fullWidth?: boolean;
}

export const TextareaField: React.FC<TextareaFieldProps> = ({
	label,
	error,
	helperText,
	fullWidth,
	id,
	className,
	...textareaProps
}) => {
	const helperTextId = id && helperText ? `${id}-helper-text` : undefined;

	return (
		<div className={cn("flex flex-col gap-1.5", fullWidth && "w-full")}>
			{label && (
				<label
					htmlFor={id}
					className={cn(
						"text-sm font-medium",
						error ? "text-content-destructive" : "text-content-primary",
					)}
				>
					{label}
				</label>
			)}
			<Textarea
				id={id}
				aria-invalid={error || undefined}
				aria-errormessage={error ? helperTextId : undefined}
				aria-describedby={!error ? helperTextId : undefined}
				className={cn(
					error &&
						"border-border-destructive focus-visible:ring-content-destructive",
					className,
				)}
				{...textareaProps}
			/>
			{helperText && (
				<p
					id={helperTextId}
					className={cn(
						"text-xs",
						error ? "text-content-destructive" : "text-content-secondary",
					)}
				>
					{helperText}
				</p>
			)}
		</div>
	);
};
