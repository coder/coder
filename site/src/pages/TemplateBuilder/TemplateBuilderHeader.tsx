import { createContext, useContext, useEffect, useRef } from "react";
import type { StepId } from "./steps";

const StepHeadingRefContext = createContext<
	React.RefCallback<HTMLHeadingElement> | undefined
>(undefined);

/** Hands the current step's heading to `useStepHeadingFocus`. */
export const StepHeadingRefProvider = StepHeadingRefContext.Provider;

function focusIfPending(
	heading: HTMLHeadingElement | null,
	pendingRef: React.RefObject<boolean>,
) {
	if (heading && pendingRef.current) {
		heading.focus({ preventScroll: true });
		pendingRef.current = false;
	}
}

/**
 * Returns a ref for `StepHeadingRefProvider` that focuses the step heading on
 * each step change, waiting for a loading step's heading to mount. Page load
 * keeps the browser's default focus.
 */
export function useStepHeadingFocus(
	stepId: StepId,
): React.RefCallback<HTMLHeadingElement> {
	const headingRef = useRef<HTMLHeadingElement | null>(null);
	const pendingRef = useRef(false);
	const handledStepIdRef = useRef(stepId);

	useEffect(() => {
		if (handledStepIdRef.current === stepId) {
			return;
		}
		handledStepIdRef.current = stepId;
		pendingRef.current = true;
		focusIfPending(headingRef.current, pendingRef);
	}, [stepId]);

	return (node: HTMLHeadingElement | null) => {
		headingRef.current = node;
		focusIfPending(node, pendingRef);
	};
}

type TemplateBuilderTitleProps = React.PropsWithChildren<{ id?: string }>;

/**
 * Step heading. `tabIndex={-1}` lets the wizard focus it on step changes
 * without adding it to the tab order.
 */
export const TemplateBuilderTitle: React.FC<TemplateBuilderTitleProps> = ({
	id,
	children,
}) => {
	const ref = useContext(StepHeadingRefContext);
	return (
		<h2
			ref={ref}
			id={id}
			tabIndex={-1}
			className="mt-0 text-xl font-semibold mb-1 outline-hidden"
		>
			{children}
		</h2>
	);
};

export const TemplateBuilderSubtitle: React.FC<React.PropsWithChildren> = ({
	children,
}) => {
	return (
		<p className="mt-0 text-sm font-normal text-content-secondary mb-4">
			{children}
		</p>
	);
};
