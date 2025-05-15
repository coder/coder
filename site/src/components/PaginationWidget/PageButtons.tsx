import { useTheme } from "@emotion/react";
import { Button } from "components/Button/Button";
import type { FC, ReactNode } from "react";
import { cn } from "utils/cn";

type NumberedPageButtonProps = {
	pageNumber: number;
	totalPages: number;

	onClick?: () => void;
	highlighted?: boolean;
	disabled?: boolean;
};

export const NumberedPageButton: FC<NumberedPageButtonProps> = ({
	pageNumber,
	totalPages,
	onClick,
	highlighted = false,
	disabled = false,
}) => {
	return (
		<BasePageButton
			name="Page button"
			aria-label={getNumberedButtonLabel(pageNumber, totalPages, highlighted)}
			onClick={onClick}
			highlighted={highlighted}
			disabled={disabled}
		>
			{pageNumber}
		</BasePageButton>
	);
};

type PlaceholderPageButtonProps = {
	pagesOmitted: number;
	children?: ReactNode;
};

export const PlaceholderPageButton: FC<PlaceholderPageButtonProps> = ({
	pagesOmitted,
	children = <>&hellip;</>,
}) => {
	return (
		<BasePageButton
			disabled
			name="Omitted pages"
			aria-label={`Omitting ${pagesOmitted} pages`}
		>
			{children}
		</BasePageButton>
	);
};

type BasePageButtonProps = {
	children?: ReactNode;
	onClick?: () => void;
	name: string;
	"aria-label": string;
	highlighted?: boolean;
	disabled?: boolean;
};

const BasePageButton: FC<BasePageButtonProps> = ({
	children,
	onClick,
	name,
	"aria-label": ariaLabel,
	highlighted = false,
	disabled = false,
}) => {
	const theme = useTheme();

	return (
		<Button
			variant="outline"
			size="sm"
			style={
				highlighted ? {
					borderColor: theme.roles.active.outline,
					backgroundColor: theme.roles.active.background,
					// Define CSS variables to use in hover styles
					"--active-border-color": theme.roles.active.outline,
					"--active-bg-color": theme.roles.active.background,
				} : undefined
			}
			className={
				highlighted ? cn(
					// Override default hover styles for highlighted buttons
					"hover:!border-[color:var(--active-border-color)] hover:!bg-[color:var(--active-bg-color)]"
				) : undefined
			}
			aria-label={ariaLabel}
			name={name}
			disabled={disabled}
			onClick={onClick}
		>
			{children}
		</Button>
	);
};

function getNumberedButtonLabel(
	page: number,
	totalPages: number,
	highlighted: boolean,
): string {
	if (highlighted) {
		return "Current Page";
	}

	if (page === 1) {
		return "First Page";
	}

	if (page === totalPages) {
		return "Last Page";
	}

	return `Page ${page}`;
}