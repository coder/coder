import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";

type PromptShellProps = HTMLAttributes<HTMLFieldSetElement> & {
	sticky?: boolean;
	disabled?: boolean;
};

export const PromptShell: FC<PromptShellProps> = ({
	sticky,
	disabled,
	className,
	children,
	...props
}) => {
	const shell = (
		<fieldset
			{...props}
			disabled={disabled}
			className={cn(
				"border border-border border-solid rounded-3xl p-3 bg-surface-secondary min-w-0 focus-within:ring-2 focus-within:ring-content-link/40",
				className,
			)}
		>
			{children}
		</fieldset>
	);

	if (sticky) {
		return (
			<div className="sticky bottom-0 z-50 bg-surface-primary">{shell}</div>
		);
	}

	return shell;
};
