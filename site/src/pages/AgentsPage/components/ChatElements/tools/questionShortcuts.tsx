import { CornerDownLeftIcon } from "lucide-react";
import { Kbd, KbdGroup } from "#/components/Kbd/Kbd";

type FormShortcut = { kind: "submit" } | { kind: "select"; rowNumber: number };

/** Modifier chords, IME composition and keys typed in the Other field keep their normal meaning. */
export const getFormShortcut = (
	event: React.KeyboardEvent,
	rowCount: number,
): FormShortcut | undefined => {
	const { target } = event;
	if (!(target instanceof HTMLElement) || target.closest("input")) {
		return undefined;
	}
	if (
		event.altKey ||
		event.ctrlKey ||
		event.metaKey ||
		event.nativeEvent.isComposing
	) {
		return undefined;
	}

	// Radix prevents Enter's default action on a radio, so the form submits.
	if (event.key === "Enter") {
		return target.getAttribute("role") === "radio"
			? { kind: "submit" }
			: undefined;
	}

	if (!/^[1-9]$/.test(event.key)) {
		return undefined;
	}
	const rowNumber = Number(event.key);
	return rowNumber <= rowCount ? { kind: "select", rowNumber } : undefined;
};

type ShortcutHintProps = {
	rowCount: number;
	enterAction: "next" | "submit";
};

/** Hidden on narrow screens and touch-only devices, which have no keys to press. */
export const ShortcutHint: React.FC<ShortcutHintProps> = ({
	rowCount,
	enterAction,
}) => (
	<KbdGroup
		aria-hidden="true"
		className="ml-auto hidden items-center gap-1.5 font-sans text-xs text-content-secondary pointer-fine:sm:inline-flex"
	>
		<Kbd>1</Kbd>to<Kbd>{rowCount}</Kbd>choose
		<Kbd className="ml-1.5">
			<CornerDownLeftIcon />
		</Kbd>
		{enterAction}
	</KbdGroup>
);
