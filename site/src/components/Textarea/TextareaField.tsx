import { cva, type VariantProps } from "class-variance-authority";
import type { ReactNode } from "react";
import { cn } from "utils/cn";
import { Textarea } from "./Textarea";

const labelVariants = cva("text-sm font-medium", {
	variants: {
		error: {
			true: "text-content-destructive",
			false: "text-content-primary",
		},
	},
	defaultVariants: { error: false },
});

const textareaVariants = cva("", {
	variants: {
		error: {
			true: "border-border-destructive focus-visible:ring-content-destructive",
			false: "",
		},
	},
	defaultVariants: { error: false },
});

const helperTextVariants = cva("text-xs", {
	variants: {
		error: {
			true: "text-content-destructive",
			false: "text-content-secondary",
		},
	},
	defaultVariants: { error: false },
});

interface SharedProps {
	label?: string;
	error?: boolean;
	helperText?: ReactNode;
	fullWidth?: boolean;
	id?: string;
	helperTextId?: string;
	className?: string;
}

// Replicates MUI's outlined + floating-label style. Uses a native <fieldset>
// with a visible <legend> so the browser naturally centers the label on the
// border without any transform magic — it scales cleanly with text size.
const MuiTextareaField: React.FC<
	SharedProps & React.ComponentPropsWithRef<"textarea">
> = ({
	label,
	error,
	helperText,
	fullWidth,
	id,
	helperTextId,
	className,
	...textareaProps
}) => {
	return (
		<div className={cn("flex flex-col gap-1.5", fullWidth && "w-full")}>
			{/* The fieldset border doubles as the outlined input border; the
			    browser draws the legend notch automatically. */}
			<fieldset
				className={cn(
					"group m-0 rounded-lg border border-solid p-0",
					error
						? "border-border-destructive"
						: "border-border focus-within:border-2 focus-within:border-content-link",
				)}
			>
				{label && (
					<legend
						className={cn(
							"ml-3 px-1 text-xs font-medium leading-none",
							error
								? "text-content-destructive"
								: "text-content-secondary group-focus-within:text-content-link",
						)}
					>
						{/* <label> inside <legend> provides click-to-focus and
						    proper form accessibility without extra ARIA. */}
						<label htmlFor={id} className="cursor-pointer">
							{label}
						</label>
					</legend>
				)}

				<Textarea
					id={id}
					aria-invalid={error || undefined}
					aria-errormessage={error ? helperTextId : undefined}
					aria-describedby={!error ? helperTextId : undefined}
					className={cn(
						// The fieldset provides the visual border; suppress the
						// default Textarea border, shadow, and focus ring.
						"border-none shadow-none focus-visible:ring-0",
						"w-full px-[14px] py-[16.5px] text-base",
						className,
					)}
					{...textareaProps}
				/>
			</fieldset>

			{helperText && (
				<p
					id={helperTextId}
					className={helperTextVariants({ error: error ?? false })}
				>
					{helperText}
				</p>
			)}
		</div>
	);
};

// Variants for the default (coderkit) layout.
const wrapperVariants = cva("flex flex-col", {
	variants: {
		variant: {
			default: "gap-1.5",
			// mui uses a separate internal component — see MuiTextareaField above.
			mui: "",
		},
	},
	defaultVariants: { variant: "default" },
});

type TextareaFieldVariantProps = VariantProps<typeof wrapperVariants>;

export interface TextareaFieldProps
	extends React.ComponentPropsWithRef<"textarea">,
		TextareaFieldVariantProps {
	label?: string;
	error?: boolean;
	helperText?: ReactNode;
	fullWidth?: boolean;
}

export const TextareaField: React.FC<TextareaFieldProps> = ({
	variant,
	label,
	error,
	helperText,
	fullWidth,
	id,
	className,
	...textareaProps
}) => {
	const helperTextId = id && helperText ? `${id}-helper-text` : undefined;

	if (variant === "mui") {
		return (
			<MuiTextareaField
				label={label}
				error={error}
				helperText={helperText}
				fullWidth={fullWidth}
				id={id}
				helperTextId={helperTextId}
				className={className}
				{...textareaProps}
			/>
		);
	}

	// Default (coderkit style) label above, border on the textarea itself.
	return (
		<div className={cn(wrapperVariants({ variant }), fullWidth && "w-full")}>
			{label && (
				<label
					htmlFor={id}
					className={labelVariants({ error: error ?? false })}
				>
					{label}
				</label>
			)}
			<Textarea
				id={id}
				aria-invalid={error || undefined}
				aria-errormessage={error ? helperTextId : undefined}
				aria-describedby={!error ? helperTextId : undefined}
				className={cn(textareaVariants({ error: error ?? false }), className)}
				{...textareaProps}
			/>
			{helperText && (
				<p
					id={helperTextId}
					className={helperTextVariants({ error: error ?? false })}
				>
					{helperText}
				</p>
			)}
		</div>
	);
};
