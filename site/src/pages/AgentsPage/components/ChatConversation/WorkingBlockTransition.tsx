import { cn } from "cn";
import {
	motion,
	type TargetAndTransition,
	type Transition,
	useReducedMotion,
} from "motion/react";
import { useState } from "react";

const LABEL_ENTRY: Transition = { duration: 0.3, ease: "easeOut" };
const FOLD_PAUSE_S = 0.06;
const FOLD_S = 0.3;
const CROSSFADE: Transition = { duration: 0.15, ease: "linear" };

const foldTransition = (delay: number): Transition => ({
	height: { duration: FOLD_S, ease: "easeInOut", delay },
	marginTop: { duration: FOLD_S, ease: "easeInOut", delay },
	// The text is gone before the clip reaches the label.
	opacity: { duration: FOLD_S * 0.6, ease: "linear", delay },
});

const LABEL_SHOWN = { height: "auto", opacity: 1, marginBottom: "0rem" };
const FOLDED = { height: 0, marginTop: "0rem", opacity: 0 };

const getLabelStart = (reducedMotion: boolean, hasFolds: boolean) => {
	if (reducedMotion) {
		return { opacity: 0 };
	}
	// The text sat one item gap under the previous row, so the label starts
	// inside that gap and the text does not jump when it mounts.
	return { height: 0, opacity: 0, marginBottom: hasFolds ? "-0.5rem" : "0rem" };
};

type FoldingReply = { id: number; content: React.ReactNode };

type WorkingBlockTransitionProps = {
	/** The label slides in above any reply text before the text folds. */
	labelEntering: boolean;
	/** Reply text folding under the label, oldest first. */
	folds: readonly FoldingReply[];
	isLive: boolean;
	onLabelEntered: () => void;
	onFolded: (id: number) => void;
	children: React.ReactNode;
};

/** Slides a block's label in above reply text, then folds the text under it. */
export const WorkingBlockTransition: React.FC<WorkingBlockTransitionProps> = ({
	labelEntering,
	folds,
	isLive,
	onLabelEntered,
	onFolded,
	children,
}) => {
	// MotionConfig's reducedMotion only skips transform and layout animations,
	// so the reduced variant here is chosen explicitly: one crossfade.
	const reducedMotion = useReducedMotion() ?? false;
	const [announced, setAnnounced] = useState(false);
	// The fold that follows a label entry waits a beat after it.
	const [pauseNextFold, setPauseNextFold] = useState(false);
	const headFold = labelEntering && !reducedMotion ? undefined : folds[0]?.id;

	return (
		<>
			<motion.div
				// Only read on mount, which is when a new live block's label enters.
				initial={
					labelEntering && getLabelStart(reducedMotion, folds.length > 0)
				}
				animate={reducedMotion ? { opacity: 1 } : LABEL_SHOWN}
				transition={reducedMotion ? CROSSFADE : LABEL_ENTRY}
				className={cn(labelEntering && !reducedMotion && "overflow-hidden")}
				onAnimationComplete={() => {
					if (!labelEntering) {
						return;
					}
					setAnnounced(true);
					setPauseNextFold(!reducedMotion);
					onLabelEntered();
				}}
			>
				{children}
			</motion.div>
			{folds.map((fold) => {
				const folding = fold.id === headFold;
				let folded: TargetAndTransition | undefined;
				if (folding) {
					folded = reducedMotion ? { opacity: 0 } : FOLDED;
				}
				return (
					<motion.div
						key={fold.id}
						// Starts as streamed; height and margin are read from the DOM.
						initial={{ opacity: 1 }}
						animate={folded}
						transition={
							reducedMotion
								? CROSSFADE
								: foldTransition(pauseNextFold ? FOLD_PAUSE_S : 0)
						}
						onAnimationComplete={() => {
							if (folding) {
								setPauseNextFold(false);
								onFolded(fold.id);
							}
						}}
						// The text already reached screen readers as it streamed.
						aria-hidden
						inert
						className="mt-2 overflow-hidden"
					>
						{fold.content}
					</motion.div>
				);
			})}
			{isLive && (
				<span role="status" className="sr-only">
					{announced ? "Working" : ""}
				</span>
			)}
		</>
	);
};
