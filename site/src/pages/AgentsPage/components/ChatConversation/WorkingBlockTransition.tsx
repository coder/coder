import { useEffectEvent, useLayoutEffect, useRef, useState } from "react";

const LABEL_ENTRY_MS = 300;
const FOLD_PAUSE_MS = 60;
const FOLD_MS = 300;
/** The text is gone before the clip reaches the label. */
const FOLD_FADE_MS = FOLD_MS * 0.6;
const REDUCED_MOTION_MS = 150;

const prefersReducedMotion = () =>
	window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// Not CSS transitions: the global reduced-motion rule cuts them to 0.01ms.
const runAnimations = (
	element: HTMLElement,
	animations: readonly [Keyframe[], KeyframeAnimationOptions][],
	onDone: () => void,
): (() => void) => {
	if (typeof element.animate !== "function") {
		onDone();
		return () => {};
	}

	const running = animations.map(([keyframes, options]) =>
		element.animate(keyframes, { fill: "forwards", ...options }),
	);
	let pending = running.length;
	for (const animation of running) {
		animation.onfinish = () => {
			pending -= 1;
			if (pending === 0) {
				onDone();
			}
		};
	}
	return () => {
		for (const animation of running) {
			animation.onfinish = null;
			animation.cancel();
		}
	};
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
	const labelRef = useRef<HTMLDivElement>(null);
	const foldRef = useRef<HTMLDivElement>(null);
	const [announced, setAnnounced] = useState(false);
	const [reducedMotion] = useState(prefersReducedMotion);
	const handleLabelEntered = useEffectEvent(onLabelEntered);
	const handleFolded = useEffectEvent(onFolded);

	useLayoutEffect(() => {
		const label = labelRef.current;
		if (!labelEntering || !label) {
			return;
		}

		let pause: number | undefined;
		const done = () => {
			setAnnounced(true);
			if (reducedMotion) {
				handleLabelEntered();
			} else {
				pause = window.setTimeout(handleLabelEntered, FOLD_PAUSE_MS);
			}
		};

		if (reducedMotion) {
			const cancel = runAnimations(
				label,
				[
					[
						[{ opacity: 0 }, { opacity: 1 }],
						{ duration: REDUCED_MOTION_MS, fill: "none" },
					],
				],
				done,
			);
			return () => {
				cancel();
				window.clearTimeout(pause);
			};
		}

		const height = label.offsetHeight;
		// The text below sat one item gap under the previous row; start the
		// label inside that gap so the text does not jump when the label mounts.
		const gap = foldRef.current
			? getComputedStyle(foldRef.current).marginTop
			: "0px";
		label.classList.add("overflow-hidden");
		const cancel = runAnimations(
			label,
			[
				[
					[
						{ height: "0px", marginBottom: `-${gap}`, opacity: 0 },
						{ height: `${height}px`, marginBottom: "0px", opacity: 1 },
					],
					// Ends at the natural height, so the label can follow its
					// content again once the animation lets go.
					{ duration: LABEL_ENTRY_MS, easing: "ease-out", fill: "none" },
				],
			],
			() => {
				label.classList.remove("overflow-hidden");
				done();
			},
		);
		return () => {
			cancel();
			label.classList.remove("overflow-hidden");
			window.clearTimeout(pause);
		};
	}, [labelEntering, reducedMotion]);

	// Under reduced motion both beats run together as one crossfade.
	const foldId = labelEntering && !reducedMotion ? undefined : folds[0]?.id;

	useLayoutEffect(() => {
		const element = foldRef.current;
		if (foldId === undefined || !element) {
			return;
		}

		const finish = () => handleFolded(foldId);
		if (reducedMotion) {
			return runAnimations(
				element,
				[[[{ opacity: 1 }, { opacity: 0 }], { duration: REDUCED_MOTION_MS }]],
				finish,
			);
		}

		const height = element.offsetHeight;
		const { marginTop } = getComputedStyle(element);
		return runAnimations(
			element,
			[
				[
					[
						{ height: `${height}px`, marginTop },
						{ height: "0px", marginTop: "0px" },
					],
					{ duration: FOLD_MS, easing: "ease-in-out" },
				],
				[[{ opacity: 1 }, { opacity: 0 }], { duration: FOLD_FADE_MS }],
			],
			finish,
		);
	}, [foldId, reducedMotion]);

	return (
		<>
			<div ref={labelRef}>{children}</div>
			{folds.map((fold, index) => (
				<div
					key={fold.id}
					ref={index === 0 ? foldRef : undefined}
					// The text already reached screen readers as it streamed.
					aria-hidden
					inert
					className="mt-2 overflow-hidden"
				>
					{fold.content}
				</div>
			))}
			{isLive && (
				<span role="status" className="sr-only">
					{announced ? "Working" : ""}
				</span>
			)}
		</>
	);
};
