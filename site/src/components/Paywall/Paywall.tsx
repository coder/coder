import type { Interpolation, Theme } from "@emotion/react";
import { Button } from "components/Button/Button";
import { CircleCheckBigIcon } from "lucide-react";
import type React from "react";
import type { FC } from "react";
import { cn } from "utils/cn";

export const Paywall = ({
	className,
	children,
	...props
}: React.ComponentProps<"div">) => {
	return (
		<div
			css={styles.root}
			className={cn(
				"flex flex-row items-center justify-center min-h-[280px] p-4 rounded-md gap-8",
				className,
			)}
			{...props}
		>
			{children}
		</div>
	);
};

export const PaywallContent: FC<React.ComponentProps<"div">> = ({
	children,
	...props
}) => {
	return <div {...props}>{children}</div>;
};

export const PaywallHeading: FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...props
}) => {
	return (
		<div
			className={cn("flex flex-row gap-4 items-center mb-6", className)}
			{...props}
		>
			{children}
		</div>
	);
};

export const PaywallTitle: FC<React.ComponentProps<"h5">> = ({
	children,
	className,
	...props
}) => {
	return (
		<h5
			className={cn("font-semibold font-inherit text-xl m-0", className)}
			{...props}
		>
			{children}
		</h5>
	);
};

export const PaywallDescription: FC<React.ComponentProps<"p">> = ({
	children,
	className,
	...props
}) => {
	return (
		<p
			className={cn("font-inherit max-w-md text-sm mb-4", className)}
			{...props}
		>
			{children}
		</p>
	);
};

export const PaywallDocumentationLink: FC<React.ComponentProps<"a">> = ({
	children = "Read the documentation",
	className,
	href,
	...props
}) => {
	return (
		<a
			href={href}
			target="_blank"
			rel="noreferrer"
			className={cn("text-content-link font-medium", className)}
			{...props}
		>
			{children}
		</a>
	);
};

export const PaywallSeparator: FC<React.ComponentProps<"hr">> = ({
	className,
	...props
}) => {
	return (
		<hr
			className={cn(
				"w-0 h-[220px] border-0 border-l border-highlight-purple/50 ml-2 mr-0",
				className,
			)}
			{...props}
		/>
	);
};

export const PaywallStack: FC<React.ComponentProps<"div">> = ({
	children,
	className,
	...props
}) => {
	return (
		<div className={cn("flex flex-col gap-6", className)} {...props}>
			{children}
		</div>
	);
};

export const PaywallFeatures: FC<React.ComponentProps<"ul">> = ({
	children,
	className,
	...props
}) => {
	const defaultFeatures: Array<{
		text: string;
		link?: { href: string; text: string };
	}> = [
		{ text: "High availability & workspace proxies" },
		{ text: "Multi-org & role-based access control" },
		{ text: "24x7 global support with SLA" },
		{ text: "Unlimited Git & external auth integrations" },
	];

	const displayFeatures = features ?? defaultFeatures;
	return (
		<ul
			className={cn("list-none m-0 px-6 text-sm font-medium", className)}
			{...props}
		>
			{children}
		</ul>
	);
};

export const PaywallFeature: FC<React.ComponentProps<"li">> = ({
	children,
	className,
	...props
}) => {
	return (
		<li className={cn("flex items-center gap-2 p-[3px]", className)} {...props}>
			<FeatureIcon className="flex-shrink-0" />
			<span className="flex-1">{children}</span>
		</li>
	);
};

export const PaywallCTA: FC<React.ComponentProps<"a">> = ({
	children,
	className,
	href,
	target = "_blank",
	rel = "noreferrer",
	...props
}) => {
	return (
		<div className="px-7">
			<Button asChild>
				<a href={href} target={target} rel={rel} {...props}>
					{children}
				</a>
			</Button>
		</div>
	);
};

const FeatureIcon: FC<React.ComponentProps<"svg">> = ({
	className,
	...props
}) => {
	return (
		<CircleCheckBigIcon
			aria-hidden="true"
			className="size-icon-sm"
			css={(theme) => ({
				color: theme.branding.premium.border,
			})}
			{...props}
		/>
	);
};

const styles = {
	root: (theme) => ({
		backgroundImage: `linear-gradient(160deg, transparent, ${theme.branding.premium.background})`,
		border: `1px solid ${theme.branding.premium.border}`,
	}),
	feature: {
		display: "flex",
		alignItems: "center",
		padding: 3,
		gap: 8,
	},
} satisfies Record<string, Interpolation<Theme>>;
