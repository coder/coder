import type { PropsWithChildren, ReactNode } from "react";
import { cn } from "#/utils/cn";

type SectionProps = PropsWithChildren<{
	title: ReactNode;
	layout?: "fluid" | "fixed";
	className?: string;
}>;

export const Section: React.FC<SectionProps> = ({
	title,
	layout = "fixed",
	className,
	children,
}) => {
	return (
		<section className={cn("flex flex-col gap-6", className)}>
			<h2 className="m-0 text-xl font-medium text-content-primary">{title}</h2>
			<div className={cn(layout === "fixed" && "max-w-3xl")}>{children}</div>
		</section>
	);
};
