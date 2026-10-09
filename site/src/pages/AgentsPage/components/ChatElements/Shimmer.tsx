import { cn } from "cn";

type TextShimmerProps = {
	children: string;
	as?: React.ElementType;
	className?: string;
	duration?: number;
	/** Scales the highlight width independently of the text length. */
	spread?: number;
};

/** Animated status text with a readable reduced-motion fallback. */
export const Shimmer = ({
	children,
	as: Component = "p",
	className,
	duration = 2,
	spread = 2,
}: TextShimmerProps) => (
	<Component
		className={cn(
			"shimmer relative inline-block text-content-secondary",
			className,
		)}
		style={{
			"--shimmer-duration": `${duration}s`,
			"--shimmer-spread": `calc(${spread * 1.5}ch + ${spread * 20}px)`,
		}}
	>
		{children}
	</Component>
);
