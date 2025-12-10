import { Button } from "components/Button/Button";
import type { ButtonHTMLAttributes, ReactNode } from "react";

type PaginationNavButtonProps = Omit<
	ButtonHTMLAttributes<HTMLButtonElement>,
	| "aria-disabled"
	// Need to omit color for MUI compatibility
	| "color"
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
