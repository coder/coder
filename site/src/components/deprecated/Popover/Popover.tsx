import MuiPopover, {
	type PopoverProps as MuiPopoverProps,
	// biome-ignore lint/nursery/noRestrictedImports: This is the base component that our custom popover is based on
} from "@mui/material/Popover";
import {
	type FC,
	type HTMLAttributes,
	type PointerEvent,
	type PointerEventHandler,
	type ReactElement,
	type ReactNode,
	type RefObject,
	cloneElement,
	createContext,
	useContext,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";

type TriggerMode = "hover" | "click";

type TriggerRef = RefObject<HTMLElement>;

// Have to append ReactNode type to satisfy React's cloneElement function. It
// has absolutely no bearing on what happens at runtime
type TriggerElement = ReactNode &
	ReactElement<{
		ref: TriggerRef;
		onClick?: () => void;
	}>;

type PopoverContextValue = {
	id: string;
	open: boolean;
	setOpen: (open: boolean) => void;
	triggerRef: TriggerRef;
	mode: TriggerMode;
};

const PopoverContext = createContext<PopoverContextValue | undefined>(
	undefined,
);

type BasePopoverProps = {
	children: ReactNode;
	mode?: TriggerMode;
};

// By separating controlled and uncontrolled props, we achieve more accurate
// type inference.
type UncontrolledPopoverProps = BasePopoverProps & {
	open?: undefined;
	onOpenChange?: undefined;
};

type ControlledPopoverProps = BasePopoverProps & {
	open: boolean;
	onOpenChange: (open: boolean) => void;
};

export type PopoverProps = UncontrolledPopoverProps | ControlledPopoverProps;

/** @deprecated prefer `components.Popover` */
export const Popover: FC<PopoverProps> = (props) => {
	const hookId = useId();
	const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
	const triggerRef: TriggerRef = useRef(null);

	// Helps makes sure that popovers close properly when the user switches to
	// a different tab. This won't help with controlled instances of the
	// component, but this is basically the most we can do from here
	useEffect(() => {
		const closeOnTabSwitch = () => setUncontrolledOpen(false);
		window.addEventListener("blur", closeOnTabSwitch);
		return () => window.removeEventListener("blur", closeOnTabSwitch);
	}, []);

	const value: PopoverContextValue = {
		triggerRef,
		id: `${hookId}-popover`,
		mode: props.mode ?? "click",
		open: props.open ?? uncontrolledOpen,
		setOpen: props.onOpenChange ?? setUncontrolledOpen,
	};

	return (
		<PopoverContext.Provider value={value}>
			{props.children}
		</PopoverContext.Provider>
	);
};

export const usePopover = () => {
	const context = useContext(PopoverContext);
	if (!context) {
		throw new Error(
			"Popover compound components cannot be rendered outside the Popover component",
		);
	}
	return context;
};

type PopoverTriggerRenderProps = Readonly<{
	isOpen: boolean;
}>;

type PopoverTriggerProps = Readonly<
	Omit<HTMLAttributes<HTMLElement>, "children"> & {
		children:
			| TriggerElement
			| ((props: PopoverTriggerRenderProps) => TriggerElement);
	}
>;

/** @deprecated prefer `components.Popover.PopoverTrigger` */
export const PopoverTrigger: FC<PopoverTriggerProps> = (props) => {
	const popover = usePopover();
	const { children, onClick, onPointerEnter, onPointerLeave, ...elementProps } =
		props;

	const clickProps = {
		onClick: (event: PointerEvent<HTMLElement>) => {
			popover.setOpen(true);
			onClick?.(event);
		},
	};

	const hoverProps = {
		onPointerEnter: (event: PointerEvent<HTMLElement>) => {
			popover.setOpen(true);
			onPointerEnter?.(event);
		},
		onPointerLeave: (event: PointerEvent<HTMLElement>) => {
			popover.setOpen(false);
			onPointerLeave?.(event);
		},
	};

	const evaluatedChildren =
		typeof children === "function"
			? children({ isOpen: popover.open })
			: children;

	return cloneElement(evaluatedChildren, {
		...elementProps,
		...(popover.mode === "click" ? clickProps : hoverProps),
		"aria-haspopup": true,
		"aria-owns": popover.id,
		"aria-expanded": popover.open,
		ref: popover.triggerRef,
	});
};

type Horizontal = "left" | "right";

export type PopoverContentProps = Omit<
	MuiPopoverProps,
	"open" | "onClose" | "anchorEl"
> & {
	horizontal?: Horizontal;
};

/** @deprecated prefer `components.Popover.PopoverContent` */
export const PopoverContent: FC<PopoverContentProps> = ({
	horizontal = "left",
	onPointerEnter,
	onPointerLeave,
	...popoverProps
}) => {
	const popover = usePopover();
	const hoverMode = popover.mode === "hover";

	return (
		<MuiPopover
			disablePortal
			css={{
				// When it is on hover mode, and the mode is moving from the trigger to
				// the popover, if there is any space, the popover will be closed. I
				// found this is a limitation on how MUI structured the component. It is
				// not a big issue for now but we can re-evaluate it in the future.
				marginTop: hoverMode ? undefined : 8,
				pointerEvents: hoverMode ? "none" : undefined,
				"& .MuiPaper-root": {
					minWidth: 320,
					fontSize: 14,
					pointerEvents: hoverMode ? "auto" : undefined,
				},
			}}
			{...horizontalProps(horizontal)}
			{...modeProps(popover, onPointerEnter, onPointerLeave)}
			{...popoverProps}
			id={popover.id}
			open={popover.open}
			onClose={() => popover.setOpen(false)}
			anchorEl={popover.triggerRef.current}
		/>
	);
};

const modeProps = (
	popover: PopoverContextValue,
	externalOnPointerEnter: PointerEventHandler<HTMLDivElement> | undefined,
	externalOnPointerLeave: PointerEventHandler<HTMLDivElement> | undefined,
) => {
	if (popover.mode === "hover") {
		return {
			onPointerEnter: (event: PointerEvent<HTMLDivElement>) => {
				popover.setOpen(true);
				externalOnPointerEnter?.(event);
			},
			onPointerLeave: (event: PointerEvent<HTMLDivElement>) => {
				popover.setOpen(false);
				externalOnPointerLeave?.(event);
			},
		};
	}

	return {};
};

const horizontalProps = (horizontal: Horizontal) => {
	if (horizontal === "right") {
		return {
			anchorOrigin: {
				vertical: "bottom",
				horizontal: "right",
			},
			transformOrigin: {
				vertical: "top",
				horizontal: "right",
			},
		} as const;
	}

	return {
		anchorOrigin: {
			vertical: "bottom",
			horizontal: "left",
		},
	} as const;
};
