import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Button } from "#/components/Button/Button";

type PaginationNavButtonProps = Omit<
	ButtonHTMLAttributes<HTMLButtonElement>,
	"aria-disabled"
> & {
	// Required/narrowed versions of default props
	children: ReactNode;
	disabled: boolean;
	onClick: () => void;
	"aria-label": string;
};

export function PaginationNavButton({
	onClick,
	disabled,
	...delegatedProps
}: PaginationNavButtonProps) {
	return (
		<Button
			variant="outline"
			size="icon"
			disabled={disabled}
			onClick={onClick}
			{...delegatedProps}
		/>
	);
}
