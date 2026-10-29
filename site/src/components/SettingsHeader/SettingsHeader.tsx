import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Link } from "#/components/Link/Link";

type SettingsHeaderProps = Readonly<
	React.PropsWithChildren<{
		actions?: React.ReactNode;
		className?: string;
	}>
>;
export const SettingsHeader: React.FC<SettingsHeaderProps> = ({
	children,
	actions,
	className,
}) => {
	return (
		<hgroup
			className={cn(
				"flex flex-col justify-between items-start gap-2 pb-6 sm:flex-row",
				className,
			)}
		>
			<div className="text-sm flex flex-col gap-2 flex-1">{children}</div>
			{actions}
		</hgroup>
	);
};

type SettingsHeaderDocsLinkProps = Readonly<
	React.PropsWithChildren<{
		href: string;
		context?: string;
	}>
>;
export const SettingsHeaderDocsLink: React.FC<SettingsHeaderDocsLinkProps> = ({
	href,
	context,
	children = "View docs",
}) => {
	return (
		<Link href={href} target="_blank" rel="noreferrer">
			{children}
			{context && <span className="sr-only"> {context}</span>}
			<span className="sr-only"> (opens in new tab)</span>
		</Link>
	);
};

const titleVariants = cva("m-0 flex items-center gap-2 leading-tight", {
	variants: {
		hierarchy: {
			primary: "text-3xl font-semibold",
			secondary: "text-2xl font-medium",
			tertiary: "text-xl font-medium",
		},
	},
	defaultVariants: {
		hierarchy: "primary",
	},
});
type SettingsHeaderTitleProps = Readonly<
	React.PropsWithChildren<
		VariantProps<typeof titleVariants> & {
			level?: `h${1 | 2 | 3 | 4 | 5 | 6}`;
			tooltip?: React.ReactNode;
			className?: string;
		}
	>
>;
export const SettingsHeaderTitle: React.FC<SettingsHeaderTitleProps> = ({
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
		<div className="flex flex-row gap-2 items-center">
			<Title className={cn(titleVariants({ hierarchy }), className)}>
				{children}
			</Title>
			{tooltip}
		</div>
	);
};

type SettingsHeaderDescriptionProps = Readonly<
	React.PropsWithChildren<{
		className?: string;
	}>
>;
export const SettingsHeaderDescription: React.FC<
	SettingsHeaderDescriptionProps
> = ({ children, className }) => {
	return (
		<p
			className={cn(
				"m-0 text-content-secondary font-medium leading-6",
				className,
			)}
		>
			{children}
		</p>
	);
};
