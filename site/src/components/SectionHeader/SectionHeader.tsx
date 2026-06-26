import { cva, type VariantProps } from "class-variance-authority";
import type { FC, ReactNode } from "react";

const titleVariants = cva("m-0 text-content-primary", {
	variants: {
		level: {
			page: "text-lg font-medium",
			section: "",
		},
		variant: {
			default: "",
			spacious: "",
		},
	},
	compoundVariants: [
		{
			level: "section",
			variant: "default",
			className: "text-sm font-medium",
		},
		{
			level: "section",
			variant: "spacious",
			className: "text-xl font-semibold leading-7",
		},
	],
	defaultVariants: {
		level: "page",
		variant: "default",
	},
});

const descriptionVariants = cva("m-0 text-content-secondary", {
	variants: {
		level: {
			page: "mt-0.5 text-sm",
			section: "",
		},
		variant: {
			default: "",
			spacious: "",
		},
	},
	compoundVariants: [
		{
			level: "section",
			variant: "default",
			className: "mt-0.5 text-xs",
		},
		{
			level: "section",
			variant: "spacious",
			className: "mt-3 text-sm font-medium leading-6",
		},
	],
	defaultVariants: {
		level: "page",
		variant: "default",
	},
});

interface SectionHeaderProps
	extends Omit<VariantProps<typeof titleVariants>, "level"> {
	label: string;
	description?: string;
	badge?: ReactNode;
	action?: ReactNode;
	/** Semantic heading level. "page" renders an h2; "section" renders an h3. */
	level?: "page" | "section";
}

export const SectionHeader: FC<SectionHeaderProps> = ({
	label,
	description,
	badge,
	action,
	level = "page",
	variant = "default",
}) => {
	const Heading = level === "section" ? "h3" : "h2";

	return (
		<div className="flex items-start justify-between gap-4">
			<div className="min-w-0 flex-1">
				<div className="flex w-full items-center gap-2">
					<Heading className={titleVariants({ level, variant })}>
						{label}
					</Heading>
					{badge}
				</div>
				{description && (
					<p className={descriptionVariants({ level, variant })}>
						{description}
					</p>
				)}
			</div>
			{action}
		</div>
	);
};
