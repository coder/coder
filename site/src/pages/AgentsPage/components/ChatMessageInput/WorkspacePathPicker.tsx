import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { isAxiosError } from "axios";
import { cn } from "cn";
import { $createTextNode, $getSelection, $isRangeSelection } from "lexical";
import { useLayoutEffect, useRef, useState } from "react";
import { useQuery } from "react-query";
import type { WorkspaceAgentFindFilesResult } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import { workspaceAgentFindFiles } from "#/api/queries/workspaceFiles";
import {
	Command,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
} from "#/components/Popover/Popover";
import { useDebouncedValue } from "#/hooks/debounce";
import {
	useWorkspaceFiles,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";
import { WorkspacePathIcon } from "./FileReferenceChip";
import {
	type ActiveTypeaheadTrigger,
	type TypeaheadTriggerMatch,
	TypeaheadTriggerPlugin,
} from "./TypeaheadTriggerPlugin";
import { $createWorkspacePathNode } from "./WorkspacePathNode";

// "@" starts a reference only at a word start, so "user@host" stays text.
const parseWorkspacePathTrigger = (
	linePrefix: string,
): TypeaheadTriggerMatch | null => {
	const match = /(?:^|\s)@(\S*)$/.exec(linePrefix);
	if (!match) {
		return null;
	}
	return {
		offset: match.index + match[0].indexOf("@"),
		query: match[1] ?? "",
	};
};

const isSameTrigger = (
	a: ActiveTypeaheadTrigger | null,
	b: ActiveTypeaheadTrigger | null,
): boolean => {
	return (
		a?.nodeKey === b?.nodeKey &&
		a?.offset === b?.offset &&
		a?.query === b?.query
	);
};

type WorkspacePathPickerProps = {
	/** The composer box the menu is pinned above and sized to match. */
	anchor: HTMLElement | null;
};

/**
 * Typing "@" at a word start searches the chat's workspace files and folders
 * and replaces "@query" with a reference chip. Inert without workspace files.
 */
export const WorkspacePathPicker: React.FC<WorkspacePathPickerProps> = ({
	anchor,
}) => {
	const workspaceFiles = useWorkspaceFiles();
	if (!workspaceFiles) {
		return null;
	}
	return <PathPicker anchor={anchor} workspaceFiles={workspaceFiles} />;
};

const PathPicker: React.FC<
	WorkspacePathPickerProps & { workspaceFiles: WorkspaceFilesContextValue }
> = ({ anchor, workspaceFiles }) => {
	const { agentId, repoRoots } = workspaceFiles;
	const [editor] = useLexicalComposerContext();
	const [trigger, setTrigger] = useState<ActiveTypeaheadTrigger | null>(null);
	const [selectedIndex, setSelectedIndex] = useState(0);
	const pointerHighlightRef = useRef(false);
	const query = trigger?.query ?? "";
	const debouncedQuery = useDebouncedValue(query, 150);
	const roots = repoRoots.length > 0 ? repoRoots : [""];
	const findQuery = useQuery({
		...workspaceAgentFindFiles(agentId, roots, debouncedQuery),
		enabled: trigger !== null && debouncedQuery !== "",
	});

	// Results for an older query are never shown, so Enter cannot insert a
	// path the typed query no longer matches.
	const isSearching =
		query !== "" && (query !== debouncedQuery || findQuery.isPending);
	let results: readonly WorkspaceAgentFindFilesResult[] = [];
	let status: string | undefined;
	if (query === "") {
		status = "Type to search files.";
	} else if (isSearching) {
		status = "Searching...";
	} else if (findQuery.data) {
		results = findQuery.data.results;
		if (results.length === 0) {
			status = "No matching files.";
		}
	} else {
		// Agents without find-files answer with a plain-text 404, which
		// getErrorStatus does not recognize as an API error.
		status =
			isAxiosError(findQuery.error) && findQuery.error.response?.status === 404
				? "Update the workspace agent to search files."
				: getErrorMessage(findQuery.error, "Failed to search files.");
	}
	const selectedResultIndex =
		results.length === 0 ? -1 : Math.min(selectedIndex, results.length - 1);
	const showRoot = roots.length > 1;

	const handleTriggerChange = (next: ActiveTypeaheadTrigger | null) => {
		if (isSameTrigger(next, trigger)) {
			return;
		}
		if (next?.query !== trigger?.query) {
			setSelectedIndex(0);
		}
		setTrigger(next);
	};

	const insertPath = (result: WorkspaceAgentFindFilesResult) => {
		const active = trigger;
		setTrigger(null);
		setSelectedIndex(0);
		if (!active) {
			return;
		}
		editor.update(() => {
			const selection = $getSelection();
			if (!$isRangeSelection(selection) || !selection.isCollapsed()) {
				return;
			}
			const { anchor: caret } = selection;
			if (caret.type !== "text" || caret.key !== active.nodeKey) {
				return;
			}
			const caretOffset = caret.offset;
			const token = caret
				.getNode()
				.getTextContent()
				.slice(active.offset, caretOffset);
			if (!/^@\S*$/.test(token)) {
				return;
			}
			selection.anchor.set(active.nodeKey, active.offset, "text");
			selection.focus.set(active.nodeKey, caretOffset, "text");
			selection.insertNodes([
				$createWorkspacePathNode(result.absolute_path, result.is_dir),
				$createTextNode(" "),
			]);
		});
	};

	const consumePointerHighlight = () => {
		const fromPointer = pointerHighlightRef.current;
		pointerHighlightRef.current = false;
		return fromPointer;
	};

	const handleHighlightedValueChange = (value: string) => {
		const nextIndex = Number(value);
		if (
			Number.isInteger(nextIndex) &&
			nextIndex >= 0 &&
			nextIndex < results.length &&
			nextIndex !== selectedResultIndex
		) {
			pointerHighlightRef.current = true;
			setSelectedIndex(nextIndex);
		}
	};

	const open = trigger !== null && anchor !== null;

	return (
		<>
			<TypeaheadTriggerPlugin
				open={trigger !== null}
				items={results}
				loading={isSearching}
				selectedIndex={selectedResultIndex}
				parseTrigger={parseWorkspacePathTrigger}
				onSelectedIndexChange={setSelectedIndex}
				onTriggerChange={handleTriggerChange}
				onSelect={insertPath}
			/>
			<Popover
				open={open}
				onOpenChange={(nextOpen) => {
					if (!nextOpen) {
						handleTriggerChange(null);
					}
				}}
			>
				{anchor && <PopoverAnchor virtualRef={{ current: anchor }} />}
				<PopoverContent
					align="start"
					side="top"
					sideOffset={8}
					className="w-(--radix-popper-anchor-width) overflow-hidden p-1 mobile-full-width-dropdown mobile-full-width-dropdown-above-composer"
					onMouseDown={(event) => event.preventDefault()}
					onOpenAutoFocus={(event) => event.preventDefault()}
					onCloseAutoFocus={(event) => event.preventDefault()}
					onEscapeKeyDown={(event) => {
						// React commits Radix's dismiss before Lexical sees the same
						// keydown, so Escape from inside the editor is left to the
						// trigger plugin, which records the dismissal.
						const rootElement = editor.getRootElement();
						if (
							event.target instanceof Node &&
							rootElement?.contains(event.target)
						) {
							event.preventDefault();
						}
					}}
				>
					<Command
						shouldFilter={false}
						loop={false}
						label="Workspace files"
						onValueChange={handleHighlightedValueChange}
						value={selectedResultIndex >= 0 ? String(selectedResultIndex) : ""}
					>
						<CommandList className="max-h-72 border-t-0 mobile-full-width-dropdown-scroll-area">
							{status !== undefined && (
								<div className="px-2 py-1.5 text-sm text-content-secondary">
									{status}
								</div>
							)}
							{results.map((result, index) => (
								<WorkspacePathOption
									key={result.absolute_path}
									result={result}
									value={String(index)}
									selected={index === selectedResultIndex}
									showRoot={showRoot}
									onSelect={insertPath}
									consumePointerHighlight={consumePointerHighlight}
								/>
							))}
						</CommandList>
					</Command>
				</PopoverContent>
			</Popover>
		</>
	);
};

const WorkspacePathOption: React.FC<{
	result: WorkspaceAgentFindFilesResult;
	value: string;
	selected: boolean;
	showRoot: boolean;
	onSelect: (result: WorkspaceAgentFindFilesResult) => void;
	consumePointerHighlight: () => boolean;
}> = ({
	result,
	value,
	selected,
	showRoot,
	onSelect,
	consumePointerHighlight,
}) => {
	const itemRef = useRef<HTMLDivElement>(null);
	const name = getPathBasename(result.path);

	// Arrow keys reach the menu through the trigger plugin, not cmdk, so the
	// item scrolls itself into view; pointer highlights skip it, like cmdk.
	useLayoutEffect(() => {
		if (!selected || consumePointerHighlight()) {
			return;
		}
		itemRef.current?.scrollIntoView({ block: "nearest" });
	}, [selected, consumePointerHighlight]);

	return (
		<CommandItem
			ref={itemRef}
			value={value}
			className={cn(
				"gap-2",
				selected && "bg-surface-secondary text-content-primary",
			)}
			onSelect={() => onSelect(result)}
		>
			<WorkspacePathIcon name={name} isDirectory={result.is_dir} />
			<span className="shrink-0 font-semibold text-content-primary">
				{result.is_dir ? `${name}/` : name}
			</span>
			<span className="min-w-0 truncate text-xs font-normal text-content-secondary">
				{showRoot
					? `${getPathBasename(result.root)}/${result.path}`
					: result.path}
			</span>
		</CommandItem>
	);
};
