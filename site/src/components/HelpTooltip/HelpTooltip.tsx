import Link from "@mui/material/Link";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	type TooltipContentProps,
	type TooltipProps,
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
	return <Tooltip {...props} />;
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

	return (
		<HelpTooltipTrigger asChild>
			<button
				{...buttonProps}
				aria-label="More info"
				ref={ref}
				style={{
					"--icon-spacing": `${getIconSpacingFromSize(size)}px`,
				}}
				className={cn(
					"flex items-center justify-center py-1 px-0 border-none bg-transparent",
					"cursor-pointer text-inherit [&_svg]:size-[var(--icon-spacing)]",
					hoverEffect && "opacity-50 hover:opacity-75",
					buttonProps.className,
				)}
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
		<h4 className={classNames.title} {...attrs}>
			{children}
		</h4>
	);
};

export const HelpTooltipText: FC<HTMLAttributes<HTMLParagraphElement>> = ({
	children,
	...attrs
}) => {
	return (
		<p {...attrs} className={cn(classNames.text, attrs.className)}>
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
		<Link
			href={href}
			target="_blank"
			rel="noreferrer"
			className={classNames.link}
		>
			<ExternalLinkIcon className={classNames.linkIcon} />
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
			className={classNames.action}
			onClick={onClick}
		>
			<Icon className={classNames.actionIcon} />
			{children}
		</button>
	);
};

export const HelpTooltipLinksGroup: FC<PropsWithChildren> = ({ children }) => {
	return (
		<Stack spacing={1} className={classNames.linksGroup}>
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

const classNames = {
	title: "mt-0 mb-2 text-content-primary text-sm leading-relaxed font-semibold",
	text: "my-1 text-sm leading-relaxed",
	link: "flex items-center text-sm text-content-link",
	linkIcon: "size-icon-xs mr-2 text-inherit",
	linksGroup: "mt-4",
	action:
		"flex items-center bg-transparent border-none p-0 cursor-pointer text-sm text-content-link",
	actionIcon: "text-inherit size-3.5 mr-2",
};
