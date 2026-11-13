import { Button } from "#/components/Button/Button";

type NumberedPageButtonProps = {
	pageNumber: number;
	onClick?: () => void;
	highlighted?: boolean;
	disabled?: boolean;
};

export const NumberedPageButton: React.FC<NumberedPageButtonProps> = ({
	pageNumber,
	onClick,
	highlighted = false,
	disabled = false,
}) => {
	return (
		<BasePageButton
			name="Page button"
			aria-label={`Page ${pageNumber}`}
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
	children?: React.ReactNode;
};

export const PlaceholderPageButton: React.FC<PlaceholderPageButtonProps> = ({
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
	children?: React.ReactNode;
	onClick?: () => void;
	name: string;
	"aria-label": string;
	highlighted?: boolean;
	disabled?: boolean;
};

const BasePageButton: React.FC<BasePageButtonProps> = ({
	children,
	onClick,
	name,
	"aria-label": ariaLabel,
	highlighted = false,
	disabled = false,
}) => {
	return (
		<Button
			variant={highlighted ? "default" : "outline"}
			size="icon"
			aria-label={ariaLabel}
			aria-current={highlighted ? "page" : undefined}
			name={name}
			disabled={disabled}
			onClick={onClick}
			className="min-w-8 w-auto"
		>
			{children}
		</Button>
	);
};
