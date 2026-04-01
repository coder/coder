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

export const HelpPopoverTrigger = PopoverTrigger;

export const HelpPopoverIcon = CircleHelpIcon;

export const HelpPopover = Popover;

export const HelpPopoverContent: FC<PopoverContentProps> = ({
	className,
	...props
}) => {
	return (
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
};

type HelpPopoverIconTriggerProps = React.ComponentPropsWithRef<"button"> & {
	size?: Size;
	hoverEffect?: boolean;
};

export const HelpPopoverIconTrigger: React.FC<HelpPopoverIconTriggerProps> = ({
	size = "medium",
	children = <HelpPopoverIcon />,
	hoverEffect = true,
	className,
	...buttonProps
}) => {
	return (
		<HelpPopoverTrigger asChild>
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
		</HelpPopoverTrigger>
	);
};

export const HelpPopoverTitle: FC<HTMLAttributes<HTMLHeadingElement>> = ({
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

export const HelpPopoverText: FC<HTMLAttributes<HTMLParagraphElement>> = ({
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

interface HelpPopoverLink {
	children?: ReactNode;
	href: string;
}

export const HelpPopoverLink: FC<HelpPopoverLink> = ({ children, href }) => {
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

interface HelpPopoverActionProps {
	children?: ReactNode;
	icon: Icon;
	onClick: () => void;
	ariaLabel?: string;
}

export const HelpPopoverAction: FC<HelpPopoverActionProps> = ({
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

export const HelpPopoverLinksGroup: FC<PropsWithChildren> = ({ children }) => {
	return <div className="flex flex-col gap-2 mt-4">{children}</div>;
};
