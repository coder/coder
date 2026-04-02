import { cva, type VariantProps } from "class-variance-authority";
import { ChevronDownIcon } from "lucide-react";
import {
	type ComponentProps,
	createContext,
	type FC,
	type ReactNode,
	useContext,
	useState,
} from "react";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { cn } from "#/utils/cn";

// ---------------------------------------------------------------------------
// Variant styles
// ---------------------------------------------------------------------------

const wrapperStyles = cva("", {
	variants: {
		variant: {
			card: "rounded-lg border border-solid border-border-default",
			inline: "border-0 border-t border-solid border-border-default pt-4",
		},
	},
	defaultVariants: { variant: "card" },
});

const triggerStyles = cva(
	"flex w-full cursor-pointer items-start justify-between gap-4 border-0 bg-transparent text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
	{
		variants: {
			variant: {
				card: "rounded-lg px-6 py-5",
				inline: "rounded-md p-0",
			},
		},
		defaultVariants: { variant: "card" },
	},
);

const titleStyles = cva("m-0 text-content-primary", {
	variants: {
		variant: {
			card: "text-lg font-semibold",
			inline: "text-sm font-medium",
		},
	},
	defaultVariants: { variant: "card" },
});

const descriptionStyles = cva("m-0 text-content-secondary", {
	variants: {
		variant: {
			card: "mt-1 text-sm",
			inline: "text-xs",
		},
	},
	defaultVariants: { variant: "card" },
});

const contentStyles = cva("", {
	variants: {
		variant: {
			card: "border-0 border-t border-solid border-border-default px-6 pb-5 pt-4",
			inline: "pt-3",
		},
	},
	defaultVariants: { variant: "card" },
});

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

type Variant = "card" | "inline";

interface CollapsibleSectionContextValue {
	variant: Variant;
	open: boolean;
}

const CollapsibleSectionContext = createContext<CollapsibleSectionContextValue>(
	{
		variant: "card",
		open: true,
	},
);

const useCollapsibleSection = () => useContext(CollapsibleSectionContext);

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

interface CollapsibleSectionProps extends VariantProps<typeof wrapperStyles> {
	defaultOpen?: boolean;
	/** Controlled open state. */
	open?: boolean;
	onOpenChange?: (open: boolean) => void;
	children: ReactNode;
}

export const CollapsibleSection: FC<CollapsibleSectionProps> = ({
	defaultOpen,
	open: controlledOpen,
	onOpenChange: controlledOnOpenChange,
	variant = "card",
	children,
}) => {
	const [uncontrolledOpen, setUncontrolledOpen] = useState(defaultOpen ?? true);
	const isControlled = controlledOpen !== undefined;
	const open = isControlled ? controlledOpen : uncontrolledOpen;
	const onOpenChange = isControlled
		? controlledOnOpenChange
		: setUncontrolledOpen;

	return (
		<CollapsibleSectionContext.Provider
			value={{ variant: variant ?? "card", open }}
		>
			<Collapsible open={open} onOpenChange={onOpenChange}>
				<div className={wrapperStyles({ variant })}>{children}</div>
			</Collapsible>
		</CollapsibleSectionContext.Provider>
	);
};

// ---------------------------------------------------------------------------
// Header (trigger)
// ---------------------------------------------------------------------------

interface CollapsibleSectionHeaderProps {
	children: ReactNode;
}

export const CollapsibleSectionHeader: FC<CollapsibleSectionHeaderProps> = ({
	children,
}) => {
	const { variant, open } = useCollapsibleSection();

	return (
		<CollapsibleTrigger className={triggerStyles({ variant })}>
			<div className="min-w-0 flex-1">{children}</div>
			<ChevronDownIcon
				className={cn(
					"h-4 w-4 shrink-0 text-content-secondary transition-transform duration-200",
					open && "rotate-180",
				)}
			/>
		</CollapsibleTrigger>
	);
};

// ---------------------------------------------------------------------------
// Title — renders the heading element at whatever level the consumer picks.
// ---------------------------------------------------------------------------

type HeadingTag = "h1" | "h2" | "h3" | "h4" | "h5" | "h6";

interface CollapsibleSectionTitleProps extends ComponentProps<HeadingTag> {
	as?: HeadingTag;
}

export const CollapsibleSectionTitle: FC<CollapsibleSectionTitleProps> = ({
	as: Component = "h2",
	className,
	...props
}) => {
	const { variant } = useCollapsibleSection();
	return (
		<Component className={cn(titleStyles({ variant }), className)} {...props} />
	);
};

// ---------------------------------------------------------------------------
// Description
// ---------------------------------------------------------------------------

export const CollapsibleSectionDescription: FC<ComponentProps<"p">> = ({
	className,
	...props
}) => {
	const { variant } = useCollapsibleSection();
	return (
		<p className={cn(descriptionStyles({ variant }), className)} {...props} />
	);
};

// ---------------------------------------------------------------------------
// Content
// ---------------------------------------------------------------------------

interface CollapsibleSectionContentProps {
	children: ReactNode;
}

export const CollapsibleSectionContent: FC<CollapsibleSectionContentProps> = ({
	children,
}) => {
	const { variant } = useCollapsibleSection();

	return (
		<CollapsibleContent>
			<div className={contentStyles({ variant })}>{children}</div>
		</CollapsibleContent>
	);
};
