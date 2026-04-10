import type { FC, ReactNode } from "react";

interface SectionHeaderProps {
	label: string;
	description?: string;
	badge?: ReactNode;
	action?: ReactNode;
	/** Controls heading size. "page" (default) renders a larger h2,
	 *  "section" renders a smaller h3 for sub-sections. */
	level?: "page" | "section";
}

export const SectionHeader: FC<SectionHeaderProps> = ({
	label,
	description,
	badge,
	action,
	level = "page",
}) => {
	const Heading = level === "section" ? "h3" : "h2";
	const headingClass =
		level === "section"
			? "m-0 text-sm font-medium text-content-primary"
			: "m-0 text-lg font-medium text-content-primary";
	const descriptionClass =
		level === "section"
			? "m-0 mt-0.5 text-xs text-content-secondary"
			: "m-0 mt-0.5 text-sm text-content-secondary";

	return (
		<>
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0 flex-1">
					<div className="flex w-full items-center gap-2">
						<Heading className={headingClass}>{label}</Heading>
						{badge}
					</div>
					{description && <p className={descriptionClass}>{description}</p>}
				</div>
				{action}
			</div>
			<hr className="my-4 border-0 border-t border-solid border-border" />
		</>
	);
};
