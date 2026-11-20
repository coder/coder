import { animate, MotionConfig, motion, useMotionValue } from "motion/react";
import { useEffect, useEffectEvent, useLayoutEffect, useRef } from "react";
import { ToolCall } from "../ChatElements/tools/ToolCall";

const ROW_HEIGHT = 24;
const ROW_GAP = 8;
const ROW_PITCH = ROW_HEIGHT + ROW_GAP;
/** The gap between the summary row and the first row, as expanded. */
const RAIL_GAP = 6;
/** A row's 16px icon and its text keep this far clear of its edges. */
const ROW_INK_INSET = (ROW_HEIGHT - 16) / 2;
/** Two full rows and a third, older one fading out at the top. */
const MAX_HEIGHT = RAIL_GAP + 3 * ROW_HEIGHT + 2 * ROW_GAP;
const TWO_ROWS_HEIGHT = RAIL_GAP + 2 * ROW_HEIGHT + ROW_GAP;
/**
 * Rows fade as they slide past the window's edges, in the rows' empty
 * margins, so rows at rest keep full contrast. `--peek-top` is the top
 * fade's height, which covers the whole third row once one shows.
 */
const TOP_FADE = RAIL_GAP + ROW_INK_INSET;
const edgeMask = `linear-gradient(to bottom, transparent, #000 var(--peek-top), #000 calc(100% - ${ROW_INK_INSET}px), transparent)`;

/**
 * Rows and the window share one curve that lands, so they arrive together
 * without a creeping tail.
 */
const slide = { duration: 0.3, ease: [0.2, 0.8, 0.2, 1] } as const;

type WorkingBlockPeekProps = {
	/** Whether the window grows in from nothing instead of appearing. */
	growIn: boolean;
	onExpand: () => void;
	/** The block's newest slices, as the expanded block renders them. */
	children: React.ReactNode;
};

/**
 * The newest rows of a collapsed live working block: its content in one-line
 * rows, clipped to a window over the content's end. When the content grows,
 * the window grows to three rows and then slides the content up, so a new row
 * enters from below.
 */
export const WorkingBlockPeek: React.FC<WorkingBlockPeekProps> = ({
	growIn,
	onExpand,
	children,
}) => {
	const trackRef = useRef<HTMLDivElement>(null);
	const height = useMotionValue(0);
	const y = useMotionValue(0);
	const topFade = useMotionValue(`${TOP_FADE}px`);
	const placed = useRef<{ height: number; y: number; fade: string } | null>(
		null,
	);
	// Where each rendered slice sat in the track at the last placement.
	const sliceTops = useRef(new Map<HTMLElement, number>());

	const place = useEffectEvent(() => {
		const track = trackRef.current;
		if (!track) {
			return;
		}
		const full = track.offsetHeight;
		// The content's top margin alone is not a row.
		const windowHeight = full <= RAIL_GAP ? 0 : Math.min(full, MAX_HEIGHT);
		const offset = windowHeight - full;
		const fade = `${full > TWO_ROWS_HEIGHT ? TOP_FADE + ROW_PITCH : TOP_FADE}px`;
		const previous = placed.current;
		placed.current = { height: windowHeight, y: offset, fade };

		// Older slices stop rendering from above the window, so the slices
		// that stay keep their place on screen.
		const slices = [...(track.firstElementChild?.children ?? [])].filter(
			(slice) => slice instanceof HTMLElement,
		);
		let shift = 0;
		for (const slice of slices) {
			const top = sliceTops.current.get(slice);
			if (top !== undefined) {
				shift = slice.offsetTop - top;
				break;
			}
		}
		sliceTops.current = new Map(
			slices.map((slice) => [slice, slice.offsetTop]),
		);

		if (!previous) {
			y.jump(offset);
			topFade.jump(fade);
			if (growIn) {
				animate(height, windowHeight, slide);
			} else {
				height.jump(windowHeight);
			}
			return;
		}
		// Only a new target starts a motion, so a stream of renders does not
		// keep restarting one.
		if (previous.height !== windowHeight) {
			animate(height, windowHeight, slide);
		}
		if (shift !== 0) {
			y.jump(y.get() - shift);
		}
		if (shift !== 0 ? y.get() !== offset : previous.y !== offset) {
			animate(y, offset, slide);
		}
		if (previous.fade !== fade) {
			animate(topFade, fade, slide);
		}
	});

	useLayoutEffect(() => {
		place();
	});

	// Rows also resize on their own state (a label, streaming reasoning), so
	// a resize observer places the window too.
	useEffect(() => {
		const track = trackRef.current;
		if (!track) {
			return;
		}
		const observer = new ResizeObserver(() => place());
		observer.observe(track);
		return () => observer.disconnect();
	}, []);

	return (
		<MotionConfig reducedMotion="user">
			{/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events -- A mouse shortcut; the summary row expands the block from the keyboard. */}
			<motion.div
				onClick={onExpand}
				className="relative cursor-pointer overflow-hidden hover:[&_.text-content-secondary]:text-content-primary"
				style={{
					height,
					"--peek-top": topFade,
					maskImage: edgeMask,
					WebkitMaskImage: edgeMask,
				}}
			>
				<motion.div ref={trackRef} className="flex flex-col" style={{ y }}>
					<ToolCall.SummaryProvider>{children}</ToolCall.SummaryProvider>
				</motion.div>
			</motion.div>
		</MotionConfig>
	);
};
