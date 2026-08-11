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
import { parsePersonalSkillTrigger } from "../../utils/personalSkills";
import type { CaretAnchorRect, SkillMenuItem } from "./SkillsTriggerMenu";

export type ActiveSkillsTrigger = {
	nodeKey: NodeKey;
	slashOffset: number;
	query: string;
	anchorRect: CaretAnchorRect | null;
};

type DismissedSkillsTrigger = Pick<
	ActiveSkillsTrigger,
	"nodeKey" | "slashOffset"
>;

type SkillsTriggerPluginProps = {
	open: boolean;
	skills: readonly SkillMenuItem[];
	selectedIndex: number;
	onSelectedIndexChange: (index: number) => void;
	onTriggerChange: (trigger: ActiveSkillsTrigger | null) => void;
	onSkillSelect: (skill: SkillMenuItem) => void;
};

const currentCaretRect = (): CaretAnchorRect | null => {
	const selection = getSelection();
	if (!selection || selection.rangeCount === 0) {
		return null;
	}

	const range = selection.getRangeAt(0);
	let rect = range.getBoundingClientRect();
	if ((rect.width === 0 && rect.height === 0) || Number.isNaN(rect.top)) {
		const fallbackRange = range.cloneRange();
		if (fallbackRange.startOffset > 0) {
			fallbackRange.setStart(
				fallbackRange.startContainer,
				fallbackRange.startOffset - 1,
			);
		}
		rect = fallbackRange.getBoundingClientRect();
	}

	if (Number.isNaN(rect.top)) {
		return null;
	}

	return {
		top: rect.top,
		left: rect.left,
		height: rect.height,
	};
};

const isSameTrigger = (
	trigger: DismissedSkillsTrigger,
	dismissedTrigger: DismissedSkillsTrigger | null,
): boolean => {
	return (
		dismissedTrigger?.nodeKey === trigger.nodeKey &&
		dismissedTrigger.slashOffset === trigger.slashOffset
	);
};

const activeTriggerFromSelection = (): Omit<
	ActiveSkillsTrigger,
	"anchorRect"
> | null => {
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
	const trigger = parsePersonalSkillTrigger(textBeforeCaret.slice(lineStart));
	if (!trigger) {
		return null;
	}

	return {
		nodeKey: node.getKey(),
		slashOffset: lineStart + trigger.slashOffset,
		query: trigger.query,
	};
};

export const SkillsTriggerPlugin = ({
	open,
	skills,
	selectedIndex,
	onSelectedIndexChange,
	onTriggerChange,
	onSkillSelect,
}: SkillsTriggerPluginProps) => {
	const [editor] = useLexicalComposerContext();
	const dismissedTriggerRef = useRef<DismissedSkillsTrigger | null>(null);

	const prevOpenRef = useRef(open);
	useLayoutEffect(() => {
		if (prevOpenRef.current && !open) {
			dismissedTriggerRef.current = editor
				.getEditorState()
				.read(() => activeTriggerFromSelection());
		}
		prevOpenRef.current = open;
	}, [open, editor]);

	const refreshTrigger = useEffectEvent(() => {
		const trigger = editor.getEditorState().read(() => {
			return editor.isEditable() ? activeTriggerFromSelection() : null;
		});

		if (!trigger) {
			dismissedTriggerRef.current = null;
			onTriggerChange(null);
			return;
		}

		if (isSameTrigger(trigger, dismissedTriggerRef.current)) {
			onTriggerChange(null);
			return;
		}

		onTriggerChange({
			...trigger,
			anchorRect: currentCaretRect(),
		});
	});

	useEffect(() => {
		return editor.registerUpdateListener(() => refreshTrigger());
	}, [editor]);

	useEffect(() => {
		return editor.registerRootListener((rootElement, previousRootElement) => {
			previousRootElement?.removeEventListener("scroll", refreshTrigger);
			rootElement?.addEventListener("scroll", refreshTrigger, {
				passive: true,
			});
		});
	}, [editor]);

	useEffect(() => {
		addEventListener("resize", refreshTrigger);
		return () => removeEventListener("resize", refreshTrigger);
	}, []);

	const moveMenuHighlight = useEffectEvent(
		(event: KeyboardEvent, delta: number) => {
			if (!open) {
				return false;
			}
			event.preventDefault();
			const count = skills.length;
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
		event?.preventDefault();
		const skill = selectedIndex >= 0 ? skills[selectedIndex] : undefined;
		if (skill) {
			onSkillSelect(skill);
		}
		return true;
	});

	const handleTab = useEffectEvent((event: KeyboardEvent | null) => {
		if (!open) {
			return false;
		}
		const skill = selectedIndex >= 0 ? skills[selectedIndex] : undefined;
		if (!skill) {
			return false;
		}
		event?.preventDefault();
		onSkillSelect(skill);
		return true;
	});

	const handleEscape = useEffectEvent((event: KeyboardEvent) => {
		if (!open) {
			return false;
		}
		event.preventDefault();
		event.stopPropagation();
		dismissedTriggerRef.current = editor
			.getEditorState()
			.read(() => activeTriggerFromSelection());
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
