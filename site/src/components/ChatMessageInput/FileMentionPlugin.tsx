import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import {
	LexicalTypeaheadMenuPlugin,
	MenuOption,
	useBasicTypeaheadTriggerMatch,
} from "@lexical/react/LexicalTypeaheadMenuPlugin";
import { API } from "api/api";
import type { TextNode } from "lexical";
import { FileIcon, FolderIcon, Loader2Icon } from "lucide-react";
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { $createFileMentionNode } from "./FileMentionNode";

class FileSearchOption extends MenuOption {
	path: string;
	fileName: string;
	isDir: boolean;

	constructor(path: string, isDir: boolean) {
		const fileName = path.split("/").pop() ?? path;
		super(path);
		this.path = path;
		this.fileName = fileName;
		this.isDir = isDir;
	}
}

interface FileMentionPluginProps {
	agentId: string;
}

const DEBOUNCE_MS = 200;

const FileMentionPlugin = memo<FileMentionPluginProps>(
	function FileMentionPlugin({ agentId }) {
		const [editor] = useLexicalComposerContext();
		const [results, setResults] = useState<FileSearchOption[]>([]);
		const [isLoading, setIsLoading] = useState(false);
		const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(
			undefined,
		);
		const abortRef = useRef<AbortController | undefined>(undefined);

		const checkForTriggerMatch = useBasicTypeaheadTriggerMatch("@", {
			minLength: 0,
			maxLength: 75,
		});

		const onQueryChange = useCallback(
			(matchingString: string | null) => {
				if (debounceRef.current) {
					clearTimeout(debounceRef.current);
				}
				if (abortRef.current) {
					abortRef.current.abort();
				}

				if (matchingString === null || matchingString.trim() === "") {
					setResults([]);
					setIsLoading(false);
					return;
				}

				setIsLoading(true);

				debounceRef.current = setTimeout(async () => {
					const controller = new AbortController();
					abortRef.current = controller;

					try {
						const response = await API.getAgentFileSearch(
							agentId,
							matchingString.trim(),
						);
						if (!controller.signal.aborted) {
							setResults(
								response.results.map(
									(r) => new FileSearchOption(r.path, r.is_dir),
								),
							);
							setIsLoading(false);
						}
					} catch {
						if (!controller.signal.aborted) {
							setResults([]);
							setIsLoading(false);
						}
					}
				}, DEBOUNCE_MS);
			},
			[agentId],
		);

		// Clean up on unmount.
		useEffect(() => {
			return () => {
				if (debounceRef.current) {
					clearTimeout(debounceRef.current);
				}
				if (abortRef.current) {
					abortRef.current.abort();
				}
			};
		}, []);

		const onSelectOption = useCallback(
			(
				selectedOption: FileSearchOption,
				nodeToReplace: TextNode | null,
				closeMenu: () => void,
			) => {
				editor.update(() => {
					const mentionNode = $createFileMentionNode(
						selectedOption.path,
						selectedOption.fileName,
					);
					if (nodeToReplace) {
						nodeToReplace.replace(mentionNode);
					}
					mentionNode.selectNext();
					closeMenu();
				});
			},
			[editor],
		);

		const options = useMemo(() => results, [results]);

		return (
			<LexicalTypeaheadMenuPlugin<FileSearchOption>
				onQueryChange={onQueryChange}
				onSelectOption={onSelectOption}
				triggerFn={checkForTriggerMatch}
				options={options}
				menuRenderFn={(
					anchorElementRef,
					{ selectedIndex, selectOptionAndCleanUp, setHighlightedIndex },
				) => {
					if (!anchorElementRef.current) {
						return null;
					}
					if (results.length === 0 && !isLoading) {
						return null;
					}

					return anchorElementRef.current
						? createPortal(
								<div className="min-w-[280px] max-w-[400px] overflow-hidden rounded-lg border border-border-default bg-surface-primary shadow-lg">
									{isLoading && results.length === 0 ? (
										<div className="flex items-center gap-2 px-3 py-2 text-sm text-content-secondary">
											<Loader2Icon className="h-4 w-4 animate-spin" />
											Searching files...
										</div>
									) : (
										<div
											role="listbox"
											className="max-h-[200px] overflow-y-auto py-1"
										>
											{options.map((option, index) => (
												<div
													key={option.key}
													role="option"
													tabIndex={-1}
													aria-selected={selectedIndex === index}
													className={`flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm ${
														selectedIndex === index
															? "bg-surface-secondary text-content-primary"
															: "text-content-secondary hover:bg-surface-secondary/50"
													}`}
													onMouseEnter={() => setHighlightedIndex(index)}
													onClick={() => {
														selectOptionAndCleanUp(option);
													}}
													onKeyDown={(e) => {
														if (e.key === "Enter" || e.key === " ") {
															e.preventDefault();
															selectOptionAndCleanUp(option);
														}
													}}
													ref={(el) => {
														option.setRefElement(el);
													}}
												>
													{option.isDir ? (
														<FolderIcon className="h-4 w-4 shrink-0 text-content-secondary" />
													) : (
														<FileIcon className="h-4 w-4 shrink-0 text-content-secondary" />
													)}
													<span className="truncate">{option.path}</span>
												</div>
											))}{" "}
											{isLoading && results.length > 0 && (
												<li className="flex items-center gap-2 px-3 py-1.5 text-sm text-content-secondary">
													<Loader2Icon className="h-4 w-4 animate-spin" />
													Searching...
												</li>
											)}
										</div>
									)}
								</div>,
								anchorElementRef.current,
							)
						: null;
				}}
			/>
		);
	},
);

export { FileMentionPlugin };
