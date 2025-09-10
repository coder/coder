import {
	type CSSObject,
	css,
	type Interpolation,
	type Theme,
} from "@emotion/react";
import Link from "@mui/material/Link";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	type TooltipContentProps,
	type TooltipProps,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { CircleHelpIcon, ExternalLinkIcon } from "lucide-react";
import {
	type FC,
	forwardRef,
	type HTMLAttributes,
	type PropsWithChildren,
	type ReactNode,
} from "react";
import { cn } from "utils/cn";

type Icon = typeof CircleHelpIcon;

type Size = "small" | "medium";

export const HelpTooltipTrigger = TooltipTrigger;

export const HelpTooltipIcon = CircleHelpIcon;

export const HelpTooltip: FC<TooltipProps> = (props) => {
	return (
		<TooltipProvider>
			<Tooltip delayDuration={0} {...props} />
		</TooltipProvider>
	);
};

export const HelpTooltipContent: FC<TooltipContentProps> = ({
	className,
	...props
}) => {
	return (
		<TooltipContent
			side="bottom"
			align="start"
			collisionPadding={16}
			{...props}
			className={cn(
				"w-[320px] p-5 bg-surface-secondary border-surface-quaternary text-sm",
				className,
			)}
		/>
	);
};

type HelpTooltipIconTriggerProps = HTMLAttributes<HTMLButtonElement> & {
	size?: Size;
	hoverEffect?: boolean;
};

export const HelpTooltipIconTrigger = forwardRef<
	HTMLButtonElement,
	HelpTooltipIconTriggerProps
>((props, ref) => {
	const {
		size = "medium",
		children = <HelpTooltipIcon />,
		hoverEffect = true,
		...buttonProps
	} = props;

	const hoverEffectStyles = css({
		opacity: 0.5,
		"&:hover": {
			opacity: 0.75,
		},
	});

	return (
		<HelpTooltipTrigger asChild>
			<button
				{...buttonProps}
				aria-label="More info"
				ref={ref}
				css={[
					css`
						display: flex;
						align-items: center;
						justify-content: center;
						padding: 4px 0;
						border: 0;
						background: transparent;
						cursor: pointer;
						color: inherit;

						& svg {
							width: ${getIconSpacingFromSize(size)}px;
							height: ${getIconSpacingFromSize(size)}px;
						}
					`,
					hoverEffect ? hoverEffectStyles : null,
				]}
			>
				{children}
			</button>
		</HelpTooltipTrigger>
	);
});

export const HelpTooltipTitle: FC<HTMLAttributes<HTMLHeadingElement>> = ({
	children,
	...attrs
}) => {
	return (
		<h4 css={styles.title} {...attrs}>
			{children}
		</h4>
	);
};

export const HelpTooltipText: FC<HTMLAttributes<HTMLParagraphElement>> = ({
	children,
	...attrs
}) => {
	return (
		<p css={styles.text} {...attrs}>
			{children}
		</p>
	);
};

interface HelpTooltipLink {
	children?: ReactNode;
	href: string;
}

export const HelpTooltipLink: FC<HelpTooltipLink> = ({ children, href }) => {
	return (
		<Link href={href} target="_blank" rel="noreferrer" css={styles.link}>
			<ExternalLinkIcon className="size-icon-xs" css={styles.linkIcon} />
			{children}
		</Link>
	);
};

interface HelpTooltipActionProps {
	children?: ReactNode;
	icon: Icon;
	onClick: () => void;
	ariaLabel?: string;
}

export const HelpTooltipAction: FC<HelpTooltipActionProps> = ({
	children,
	icon: Icon,
	onClick,
	ariaLabel,
}) => {
	return (
		<button
			type="button"
			aria-label={ariaLabel ?? ""}
			css={styles.action}
			onClick={onClick}
		>
			<Icon css={styles.actionIcon} />
			{children}
		</button>
	);
};

export const HelpTooltipLinksGroup: FC<PropsWithChildren> = ({ children }) => {
	return (
		<Stack spacing={1} css={styles.linksGroup}>
			{children}
		</Stack>
	);
};

const getIconSpacingFromSize = (size?: Size): number => {
	switch (size) {
		case "small":
			return 12;
		default:
			return 16;
	}
};

const styles = {
	title: (theme) => ({
		marginTop: 0,
		marginBottom: 8,
		color: theme.palette.text.primary,
		fontSize: 14,
		lineHeight: "150%",
		fontWeight: 600,
	}),

	text: (theme) => ({
		marginTop: 4,
		marginBottom: 4,
		...(theme.typography.body2 as CSSObject),
	}),

	link: (theme) => ({
		display: "flex",
		alignItems: "center",
		...(theme.typography.body2 as CSSObject),
		color: theme.roles.active.fill.outline,
	}),

	linkIcon: {
		color: "inherit",
		width: 14,
		height: 14,
		marginRight: 8,
	},

	linksGroup: {
		marginTop: 16,
	},

	action: (theme) => ({
		display: "flex",
		alignItems: "center",
		background: "none",
		border: 0,
		color: theme.palette.primary.light,
		padding: 0,
		cursor: "pointer",
		fontSize: 14,
	}),

	actionIcon: {
		color: "inherit",
		width: 14,
		height: 14,
		marginRight: 8,
	},
} satisfies Record<string, Interpolation<Theme>>;
