import { type CSSObject, useTheme } from "@emotion/react";
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
	const theme = useTheme();

	return (
		<form
			className="rounded-lg border border-solid border-zinc-700 overflow-hidden"
			onSubmit={onSubmit}
		>
			<header className="p-6">
				<div className="text-xl font-semibold m-0">{title}</div>
				{subtitle && (
					<div className="text-sm text-content-secondary mt-2">{subtitle}</div>
				)}
				<div css={[theme.typography.body2 as CSSObject, { paddingTop: 16 }]}>
					{children}
				</div>
			</header>
			<footer
				css={[
					theme.typography.body2 as CSSObject,
					{
						background: theme.palette.background.paper,
						padding: "16px 24px",
						display: "flex",
						alignItems: "center",
						justifyContent: "space-between",
					},
				]}
			>
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
