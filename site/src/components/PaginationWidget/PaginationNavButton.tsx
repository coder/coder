import { Button } from "#/components/Button/Button";

type PaginationNavButtonProps = Omit<
	React.ComponentProps<"button">,
	"aria-disabled"
> & {
	// Required/narrowed versions of default props
	children: React.ReactNode;
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
