import { Button } from "components/Button/Button";
import type { FC, FormEventHandler, JSX, ReactNode } from "react";

interface FieldsetProps {
	children: ReactNode;
	title: string | JSX.Element;
	subtitle?: string | JSX.Element;
	validation?: string | JSX.Element | false;
	button?: JSX.Element | false;
	onSubmit: FormEventHandler<HTMLFormElement>;
	isSubmitting?: boolean;
}

export const Fieldset: FC<FieldsetProps> = ({
	title,
	subtitle,
	children,
	validation,
	button,
	onSubmit,
	isSubmitting,
}) => {
	return (
		<form
			className="mt-8 overflow-hidden rounded-lg border border-solid border-border"
			onSubmit={onSubmit}
		>
			<header className="p-6">
				<div className="text-xl font-semibold">{title}</div>
				{subtitle && (
					<div className="mt-2 text-sm text-content-secondary">{subtitle}</div>
				)}
				<div className="pt-4 text-sm">{children}</div>
			</header>
			<footer className="flex items-center justify-between bg-surface-secondary px-6 py-4 text-sm">
				<div className="text-content-secondary">{validation}</div>
				{button || (
					<Button type="submit" disabled={isSubmitting}>
						Submit
					</Button>
				)}
			</footer>
		</form>
	);
};
