import { CircleHelpIcon, ExternalLinkIcon } from "lucide-react";
import type { FC, HTMLAttributes, PropsWithChildren, ReactNode } from "react";
import {
	Popover,
	PopoverContent,
	type PopoverContentProps,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

type Icon = typeof CircleHelpIcon;

type Size = "small" | "medium";

export const HelpTooltipTrigger = PopoverTrigger;

export const HelpTooltipIcon = CircleHelpIcon;

export const HelpTooltip: FC<React.ComponentProps<typeof Popover>> = (
	props,
) => {
	return <Popover {...props} />;
};

export const HelpTooltipContent: FC<
	PopoverContentProps & { disablePortal?: boolean }
> = ({ className, disablePortal, ...props }) => {
	const content = (
		<PopoverContent
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

	// PopoverContent already uses Portal internally, so disablePortal is a no-op
	// but we accept it for backward compatibility with TooltipContent API
	return content;
};

type HelpTooltipIconTriggerProps = React.ComponentPropsWithRef<"button"> & {
	size?: Size;
	hoverEffect?: boolean;
};

export const HelpTooltipIconTrigger: React.FC<HelpTooltipIconTriggerProps> = ({
	size = "medium",
	children = <HelpTooltipIcon />,
	hoverEffect = true,
	className,
	...buttonProps
}) => {
	return (
		<HelpTooltipTrigger asChild>
			<button
				{...buttonProps}
				type="button"
				aria-label="More info"
				className={cn(
					"flex items-center justify-center px-0 py-1",
					"border-0 border-none bg-transparent cursor-pointer text-inherit",
					size === "small" ? "[&_svg]:size-3" : "[&_svg]:size-4",
					hoverEffect && "opacity-50 hover:opacity-75",
					className,
				)}
			>
				{children}
			</button>
		</HelpTooltipTrigger>
	);
};

export const HelpTooltipTitle: FC<HTMLAttributes<HTMLHeadingElement>> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<h4
			className={cn(
				"mt-0 mb-2 text-content-primary text-sm leading-[150%] font-semibold",
				className,
			)}
			{...attrs}
		>
			{children}
		</h4>
	);
};

export const HelpTooltipText: FC<HTMLAttributes<HTMLParagraphElement>> = ({
	children,
	className,
	...attrs
}) => {
	return (
		<p
			className={cn(
				"my-1 text-sm text-content-secondary leading-normal",
				className,
			)}
			{...attrs}
		>
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
		<a
			href={href}
			target="_blank"
			rel="noreferrer"
			className="flex items-center text-sm text-content-link no-underline hover:underline"
		>
			<ExternalLinkIcon className="size-3.5 mr-2" />
			{children}
		</a>
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
			className="flex items-center bg-transparent border-0 border-none text-content-link p-0 cursor-pointer text-sm"
			onClick={onClick}
		>
			<Icon className="size-3.5 mr-2" />
			{children}
		</button>
	);
};

export const HelpTooltipLinksGroup: FC<PropsWithChildren> = ({ children }) => {
	return <div className="flex flex-col gap-2 mt-4">{children}</div>;
};
