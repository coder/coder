import { cn } from "cn";
import { MotionConfig, MotionConfigContext, motion } from "motion/react";
import { useContext } from "react";

const motionComponents = {
	p: motion.p,
	span: motion.span,
};

type TextShimmerProps = {
	children: string;
	as?: keyof typeof motionComponents;
	className?: string;
	duration?: number;
	spread?: number;
};

const ShimmerComponent = ({
	children,
	as = "p",
	className,
	duration = 2,
	spread = 2,
}: TextShimmerProps) => {
	const MotionComponent = motionComponents[as];

	// skipAnimations jumps to the final keyframe, which leaves the
	// gradient offscreen, so hold it centered for captures.
	const { skipAnimations = false } = useContext(MotionConfigContext);

	const dynamicSpread = (children?.length ?? 0) * spread;

	return (
		<MotionConfig reducedMotion="user">
			<MotionComponent
				animate={{
					backgroundPosition: skipAnimations ? "50% center" : "0% center",
				}}
				className={cn(
					"relative inline-block bg-size-[250%_100%,auto] bg-clip-text text-transparent",
					"[--bg:linear-gradient(90deg,#0000_calc(50%-var(--spread)),hsl(var(--surface-primary)),#0000_calc(50%+var(--spread)))] bg-no-repeat",
					className,
				)}
				initial={{ backgroundPosition: "100% center" }}
				style={{
					"--spread": `${dynamicSpread}px`,
					backgroundImage:
						"var(--bg), linear-gradient(hsl(var(--content-secondary)), hsl(var(--content-secondary)))",
				}}
				transition={{
					duration,
					ease: "linear",
					repeat: Number.POSITIVE_INFINITY,
				}}
			>
				{children}
			</MotionComponent>
		</MotionConfig>
	);
};

export const Shimmer = ShimmerComponent;
