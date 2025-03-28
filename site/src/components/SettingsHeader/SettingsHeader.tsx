import { type VariantProps, cva } from "class-variance-authority";
import { Button } from "components/Button/Button";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC, PropsWithChildren, ReactNode } from "react";
import { cn } from "utils/cn";

type SettingsHeaderProps = Readonly<
	PropsWithChildren<{
		actions?: ReactNode;
		className?: string;
	}>
>;
export const SettingsHeader: FC<SettingsHeaderProps> = ({
	children,
	actions,
	className,
}) => {
	return (
		<hgroup className="flex flex-col justify-between items-start gap-2 pb-6 sm:flex-row">
			{/*
			 * The text-sm class is only meant to adjust the font size of
			 * SettingsDescription, but we need to apply it here. That way,
			 * text-sm combines with the max-w-prose class and makes sure
			 * we have a predictable max width for the header + description by
			 * default.
			 */}
			<div className={cn("text-sm max-w-prose", className)}>{children}</div>
			{actions}
		</hgroup>
	);
};

type SettingsHeaderDocsLinkProps = Readonly<
	PropsWithChildren<{ href: string }>
>;
export const SettingsHeaderDocsLink: FC<SettingsHeaderDocsLinkProps> = ({
	href,
	children = "Read the docs",
}) => {
	return (
		<Button asChild variant="outline">
			<a href={href} target="_blank" rel="noreferrer">
				<SquareArrowOutUpRightIcon />
				{children}
				<span className="sr-only"> (link opens in new tab)</span>
			</a>
		</Button>
	);
};

const titleVariants = cva("m-0 pb-1 flex items-center gap-2 leading-tight", {
	variants: {
		hierarchy: {
			primary: "text-3xl font-bold",
			secondary: "text-2xl font-medium",
		},
	},
	defaultVariants: {
		hierarchy: "primary",
	},
});
type SettingsHeaderTitleProps = Readonly<
	PropsWithChildren<
		VariantProps<typeof titleVariants> & {
			level?: `h${1 | 2 | 3 | 4 | 5 | 6}`;
			tooltip?: ReactNode;
			className?: string;
		}
	>
>;
export const SettingsHeaderTitle: FC<SettingsHeaderTitleProps> = ({
	children,
	tooltip,
	className,
	level = "h1",
	hierarchy = "primary",
}) => {
	// Explicitly not using Radix's Slot component, because we don't want to
	// allow any arbitrary element to be composed into this. We specifically
	// only want to allow the six HTML headers. Anything else will likely result
	// in invalid markup
	const Title = level;
	return (
		<div className="flex flex-row gap-2 items-middle">
			<Title className={cn(titleVariants({ hierarchy }), className)}>
				{children}
			</Title>
			{tooltip}
		</div>
	);
};

type SettingsHeaderDescriptionProps = Readonly<
	PropsWithChildren<{
		className?: string;
	}>
>;
export const SettingsHeaderDescription: FC<SettingsHeaderDescriptionProps> = ({
	children,
	className,
}) => {
	return (
		<p className={cn("m-0 text-content-secondary leading-relaxed", className)}>
			{children}
		</p>
	);
};
