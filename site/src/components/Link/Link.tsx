import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import { Slot } from "radix-ui";
import { isValidElement } from "react";

/**
 * py-1 -my-1 meets the WCAG 2.5.8 minimum pointer target size of 24px without
 * shifting layout, and the hover underline (bottom-1.25) relies on py-1. w-fit
 * keeps flex and grid parents from stretching the link past its text.
 *
 * Override with pl-0, not p-0; subtract 4px from added vertical margin, or
 * set my-0 when replacing padding.
 */
const linkVariants = cva(
	cn(
		"relative inline-flex items-center no-underline font-medium text-content-link hover:cursor-pointer",
		"w-fit py-1 -my-1",
		"hover:after:content-[''] hover:after:absolute hover:after:left-0 hover:after:w-full hover:after:h-px hover:after:bg-current hover:after:bottom-1.25",
		"focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link",
		"focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary focus-visible:rounded-sm",
		"visited:text-content-link pl-0.5",
	), //pl-0.5 adjusts the underline spacing to align with the icon on the right.
	{
		variants: {
			size: {
				lg: "text-sm gap-0.5 [&_svg]:size-icon-sm [&_svg]:p-0.5 leading-6",
				sm: "text-xs gap-1 [&_svg]:size-icon-xs [&_svg]:p-px leading-5",
			},
		},
		defaultVariants: {
			size: "lg",
		},
	},
);

export type LinkProps = React.ComponentProps<"a"> &
	VariantProps<typeof linkVariants> & {
		asChild?: boolean;
		/**
		 * Defaults to showing the icon only for links that open in a new tab or
		 * point to another origin.
		 */
		showExternalIcon?: boolean;
	};

export const Link: React.FC<LinkProps> = ({
	className,
	children,
	size,
	asChild,
	showExternalIcon,
	...props
}) => {
	const Comp = asChild ? Slot.Root : "a";
	// Slot merges Link's props into the child, and the child's win. A child
	// without href, such as a router link using `to`, counts as internal.
	const childTarget = asChild ? getChildLinkTarget(children) : {};
	const href = childTarget.href ?? props.href;
	const target = childTarget.target ?? props.target;
	const opensInNewTab = target === "_blank";
	const showIcon = showExternalIcon ?? (opensInNewTab || isExternalHref(href));
	return (
		<Comp className={cn(linkVariants({ size }), className)} {...props}>
			<Slot.Slottable>{children}</Slot.Slottable>
			{showIcon && <SquareArrowOutUpRightIcon />}
			{showIcon && opensInNewTab && <NewTabLabel />}
		</Comp>
	);
};

const getChildLinkTarget = (
	children: React.ReactNode,
): Pick<React.ComponentProps<"a">, "href" | "target"> => {
	if (!isValidElement<{ href?: unknown; target?: unknown }>(children)) {
		return {};
	}
	const { href, target } = children.props;
	return {
		href: typeof href === "string" ? href : undefined,
		target: typeof target === "string" ? target : undefined,
	};
};

const isExternalHref = (href: string | undefined): boolean => {
	if (!href) {
		return false;
	}
	// new URL instead of URL.parse, which Chrome 111 and Safari 16.4 lack.
	try {
		const url = new URL(href, location.href);
		return (
			(url.protocol === "http:" || url.protocol === "https:") &&
			url.origin !== location.origin
		);
	} catch {
		return false;
	}
};

export const NewTabLabel: React.FC = () => (
	<span className="sr-only"> (opens in new tab)</span>
);
