import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import type { FC, HTMLAttributes, ReactNode } from "react";

export const Stats: FC<HTMLAttributes<HTMLDivElement>> = ({
	children,
	...attrs
}) => {
	return (
		<div
			className="block p-4 rounded-md border-border m-0 flex-wrap text-content-secondary md:flex md:py-0"
			css={styles.stats}
			{...attrs}
		>
			{children}
		</div>
	);
};

interface StatsItemProps extends HTMLAttributes<HTMLDivElement> {
	label: string;
	value: ReactNode;
}

export const StatsItem: FC<StatsItemProps> = ({ label, value, ...attrs }) => {
	return (
		<div className="py-3.5 px-4 flex items-baseline gap-2 md:p-2" {...attrs}>
			<span className="block break-words">{label}:</span>
			<span className="mt-0.5 flex break-words text-content-primary items-center [&_a]:text-content-primary no-underline font-semibold [&_a]:hover:underline">
				{value}
			</span>
		</div>
	);
};

const styles = {
	stats: (theme) => {
		const value = theme.typography.body2;
		console.log(value);
		return value as CSSObject;
	},
} satisfies Record<string, Interpolation<Theme>>;
