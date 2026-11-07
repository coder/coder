import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox";
import { cn } from "cn";
import { CheckIcon, MinusIcon } from "lucide-react";

type CheckboxProps = Omit<
	React.ComponentProps<typeof CheckboxPrimitive.Root>,
	"className" | "checked"
> & { className?: string; checked?: boolean | "indeterminate" };

export const Checkbox: React.FC<CheckboxProps> = ({
	className,
	checked,
	...props
}) => {
	return (
		<CheckboxPrimitive.Root
			nativeButton
			render={(buttonProps) => <button {...buttonProps} type="button" />}
			checked={checked === "indeterminate" ? false : checked}
			indeterminate={checked === "indeterminate"}
			className={cn(
				"peer size-[18px] shrink-0 rounded-xs border border-border-control border-solid m-1",
				"focus-visible:outline-hidden focus-visible:ring-2 relative",
				"focus-visible:ring-content-link focus-visible:ring-offset-[3px] focus-visible:ring-offset-surface-primary",
				"disabled:cursor-not-allowed",
				"disabled:bg-surface-primary",
				"disabled:data-checked:bg-surface-tertiary",
				"disabled:data-checked:border-surface-tertiary",
				"data-unchecked:bg-surface-primary",
				"data-checked:bg-surface-invert-primary",
				"data-checked:text-content-invert",
				"data-checked:border-surface-invert-primary",
				"data-indeterminate:bg-surface-invert-primary",
				"data-indeterminate:text-content-invert",
				"data-indeterminate:border-surface-invert-primary",
				"hover:enabled:border-border-control-hover",
				"hover:data-checked:bg-surface-invert-secondary",
				"hover:data-checked:border-surface-invert-secondary",
				"hover:data-indeterminate:bg-surface-invert-secondary",
				"hover:data-indeterminate:border-surface-invert-secondary",
				className,
			)}
			{...props}
		>
			<CheckboxPrimitive.Indicator
				className={cn(
					"absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 size-4",
				)}
			>
				{checked === "indeterminate" ? (
					<MinusIcon className="size-4" strokeWidth={2.5} />
				) : (
					<CheckIcon className="size-4" strokeWidth={2.5} />
				)}
			</CheckboxPrimitive.Indicator>
		</CheckboxPrimitive.Root>
	);
};
