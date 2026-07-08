import {
	CodeIcon,
	DatabaseIcon,
	KeyIcon,
	LayoutTemplateIcon,
	PackageIcon,
	WrenchIcon,
} from "lucide-react";
import { motion } from "motion/react";
import type { FC } from "react";

type FloatingIcon = {
	name: string;
	icon: FC<{ className?: string }>;
	// Position as percentage offsets from the container center.
	x: number;
	y: number;
	delay: number;
};

const ICONS: FloatingIcon[] = [
	{ name: "package", icon: PackageIcon, x: -18, y: 20, delay: 0 },
	{ name: "key", icon: KeyIcon, x: -25, y: -10, delay: 0.4 },
	{ name: "code", icon: CodeIcon, x: 15, y: 25, delay: 0.8 },
	{ name: "database", icon: DatabaseIcon, x: 18, y: -30, delay: 1.2 },
	{ name: "wrench", icon: WrenchIcon, x: 10, y: 0, delay: 1.6 },
	{ name: "template", icon: LayoutTemplateIcon, x: 28, y: -8, delay: 2.0 },
];

/**
 * Full-area animated loader displayed while the template builder compose
 * API call is in flight. Shows floating icon boxes, a progress bar, and
 * an animated "Building your template..." label.
 */
export const BuildingTemplateLoader: FC = () => {
	return (
		<div
			className="relative flex flex-col items-center justify-end w-full min-h-[480px] overflow-hidden"
			role="status"
			aria-label="Building your template"
		>
			{/* Floating icon boxes; bottom-24 keeps them above the progress bar */}
			<div className="absolute inset-0 bottom-24">
				{ICONS.map(({ name, icon: Icon, x, y, delay }) => (
					<motion.div
						key={name}
						className="absolute rounded-md border border-solid border-border bg-surface-secondary p-3"
						style={{
							left: `calc(50% + ${x}%)`,
							top: `calc(50% + ${y}%)`,
						}}
						animate={{
							y: ["-100vh", "0vh", "0vh", "0vh", "-100vh", "-100vh"],
							opacity: [0, 1, 1, 0, 0, 0],
						}}
						transition={{
							duration: 7,
							delay,
							ease: [0.25, 0.1, 0.25, 1],
							repeat: Number.POSITIVE_INFINITY,
							times: [0, 0.25, 0.55, 0.7, 0.8, 1],
						}}
					>
						<Icon className="size-6 text-content-primary" />
					</motion.div>
				))}
			</div>

			{/* Progress bar and label */}
			<div className="relative z-10 flex w-full max-w-[480px] flex-col items-center gap-3 pb-8">
				{/* Progress track */}
				<div className="flex h-2.5 w-full items-center justify-center">
					<div className="h-2.5 w-full overflow-hidden rounded bg-surface-sky">
						<motion.div
							className="h-full rounded bg-highlight-sky"
							initial={{ width: "0%" }}
							animate={{ width: "100%" }}
							transition={{
								duration: 5,
								ease: "easeInOut",
								repeat: Number.POSITIVE_INFINITY,
							}}
						/>
					</div>
				</div>

				{/* Label */}
				<p className="flex items-center gap-0.5 text-xs leading-[18px] text-content-secondary">
					<span>Building your template</span>
					<span className="inline-flex gap-0.5">
						<motion.span
							animate={{ opacity: [0, 1, 1, 0] }}
							transition={{
								duration: 1.5,
								repeat: Number.POSITIVE_INFINITY,
								times: [0, 0.2, 0.8, 1],
							}}
						>
							.
						</motion.span>
						<motion.span
							animate={{ opacity: [0, 1, 1, 0] }}
							transition={{
								duration: 1.5,
								delay: 0.2,
								repeat: Number.POSITIVE_INFINITY,
								times: [0, 0.2, 0.8, 1],
							}}
						>
							.
						</motion.span>
						<motion.span
							animate={{ opacity: [0, 1, 1, 0] }}
							transition={{
								duration: 1.5,
								delay: 0.4,
								repeat: Number.POSITIVE_INFINITY,
								times: [0, 0.2, 0.8, 1],
							}}
						>
							.
						</motion.span>
					</span>
				</p>
			</div>
		</div>
	);
};
