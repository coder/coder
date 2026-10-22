import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import {
	$getSelection,
	$isRangeSelection,
	$isTextNode,
	COMMAND_PRIORITY_CRITICAL,
	KEY_ARROW_DOWN_COMMAND,
	KEY_ARROW_UP_COMMAND,
	KEY_ENTER_COMMAND,
	KEY_ESCAPE_COMMAND,
	KEY_TAB_COMMAND,
	type NodeKey,
} from "lexical";
import { useEffect, useEffectEvent, useLayoutEffect, useRef } from "react";

/** A trigger character and the query typed after it on the caret's line. */
export type TypeaheadTriggerMatch = {
	/** Offset of the trigger character within the parsed line prefix. */
	offset: number;
	query: string;
};

export type ActiveTypeaheadTrigger = {
	nodeKey: NodeKey;
	/** Offset of the trigger character within the text node. */
	offset: number;
	query: string;
};

type TriggerLocation = Pick<ActiveTypeaheadTrigger, "nodeKey" | "offset">;

type TypeaheadTriggerPluginProps<T> = {
	open: boolean;
	items: readonly T[];
	loading: boolean;
	selectedIndex: number;
	parseTrigger: (linePrefix: string) => TypeaheadTriggerMatch | null;
	onSelectedIndexChange: (index: number) => void;
	onTriggerChange: (trigger: ActiveTypeaheadTrigger | null) => void;
	onSelect: (item: T) => void;
};

const isSameTrigger = (
	trigger: TriggerLocation,
	dismissedTrigger: TriggerLocation | null,
): boolean => {
	return (
		dismissedTrigger?.nodeKey === trigger.nodeKey &&
		dismissedTrigger.offset === trigger.offset
	);
};

const activeTriggerFromSelection = (
	parseTrigger: (linePrefix: string) => TypeaheadTriggerMatch | null,
): ActiveTypeaheadTrigger | null => {
	const selection = $getSelection();
	if (!$isRangeSelection(selection) || !selection.isCollapsed()) {
		return null;
	}

	const anchor = selection.anchor;
	if (anchor.type !== "text") {
		return null;
	}

	const node = anchor.getNode();
	if (!$isTextNode(node)) {
		return null;
	}

	const textBeforeCaret = node.getTextContent().slice(0, anchor.offset);
	const lineStart = textBeforeCaret.lastIndexOf("\n") + 1;
	const trigger = parseTrigger(textBeforeCaret.slice(lineStart));
	if (!trigger) {
		return null;
	}

	return {
		nodeKey: node.getKey(),
		offset: lineStart + trigger.offset,
		query: trigger.query,
	};
};

/**
 * Tracks a typeahead trigger (such as "/" or "@") before the caret and routes
 * the editor's arrow, Enter, Tab, and Escape keys to its menu while it is open.
 * A dismissed trigger stays closed until its location changes.
 */
export const TypeaheadTriggerPlugin = <T,>({
	open,
	items,
	loading,
	selectedIndex,
	parseTrigger,
	onSelectedIndexChange,
	onTriggerChange,
	onSelect,
}: TypeaheadTriggerPluginProps<T>) => {
	const [editor] = useLexicalComposerContext();
	const dismissedTriggerRef = useRef<TriggerLocation | null>(null);

	const readActiveTrigger = useEffectEvent(() =>
		editor
			.getEditorState()
			.read(() => activeTriggerFromSelection(parseTrigger)),
	);

	const prevOpenRef = useRef(open);
	useLayoutEffect(() => {
		if (prevOpenRef.current && !open) {
			dismissedTriggerRef.current = readActiveTrigger();
		}
		prevOpenRef.current = open;
	}, [open]);

	const refreshTrigger = useEffectEvent(() => {
		const trigger = editor.isEditable() ? readActiveTrigger() : null;

		if (!trigger) {
			dismissedTriggerRef.current = null;
			onTriggerChange(null);
			return;
		}

		if (isSameTrigger(trigger, dismissedTriggerRef.current)) {
			onTriggerChange(null);
			return;
		}

		onTriggerChange(trigger);
	});

	useEffect(() => {
		return editor.registerUpdateListener(() => refreshTrigger());
	}, [editor]);

	const moveMenuHighlight = useEffectEvent(
		(event: KeyboardEvent, delta: number) => {
			if (!open) {
				return false;
			}
			event.preventDefault();
			const count = items.length;
			if (count === 0) {
				return true;
			}
			if (selectedIndex < 0) {
				onSelectedIndexChange(delta > 0 ? 0 : count - 1);
				return true;
			}
			onSelectedIndexChange((selectedIndex + delta + count) % count);
			return true;
		},
	);

	const handleEnter = useEffectEvent((event: KeyboardEvent | null) => {
		if (!open) {
			return false;
		}
		const item = selectedIndex >= 0 ? items[selectedIndex] : undefined;
		if (!item) {
			// A still-loading source may yet produce matches, so keep
			// consuming Enter until every source resolves.
			if (loading) {
				event?.preventDefault();
				return true;
			}
			// Nothing is selectable (e.g. the trailing token is a filesystem
			// path, not a skill): dismiss the menu and let the same keypress
			// fall through to the submit handler.
			dismissedTriggerRef.current = readActiveTrigger();
			onTriggerChange(null);
			return false;
		}
		event?.preventDefault();
		onSelect(item);
		return true;
	});

	const handleTab = useEffectEvent((event: KeyboardEvent | null) => {
		if (!open) {
			return false;
		}
		const item = selectedIndex >= 0 ? items[selectedIndex] : undefined;
		if (!item) {
			return false;
		}
		event?.preventDefault();
		onSelect(item);
		return true;
	});

	const handleEscape = useEffectEvent((event: KeyboardEvent) => {
		if (!open) {
			return false;
		}
		event.preventDefault();
		event.stopPropagation();
		dismissedTriggerRef.current = readActiveTrigger();
		onTriggerChange(null);
		const rootElement = editor.getRootElement();
		queueMicrotask(() => {
			if (rootElement?.isConnected) {
				editor.focus();
			}
		});
		return true;
	});

	useEffect(() => {
		const unregisterArrowDown = editor.registerCommand(
			KEY_ARROW_DOWN_COMMAND,
			(event: KeyboardEvent) => moveMenuHighlight(event, 1),
			COMMAND_PRIORITY_CRITICAL,
		);
		const unregisterArrowUp = editor.registerCommand(
			KEY_ARROW_UP_COMMAND,
			(event: KeyboardEvent) => moveMenuHighlight(event, -1),
			COMMAND_PRIORITY_CRITICAL,
		);
		const unregisterEnter = editor.registerCommand(
			KEY_ENTER_COMMAND,
			handleEnter,
			COMMAND_PRIORITY_CRITICAL,
		);
		const unregisterTab = editor.registerCommand(
			KEY_TAB_COMMAND,
			handleTab,
			COMMAND_PRIORITY_CRITICAL,
		);
		const unregisterEscape = editor.registerCommand(
			KEY_ESCAPE_COMMAND,
			handleEscape,
			COMMAND_PRIORITY_CRITICAL,
		);

		return () => {
			unregisterArrowDown();
			unregisterArrowUp();
			unregisterEnter();
			unregisterTab();
			unregisterEscape();
		};
	}, [editor]);

	return null;
};
