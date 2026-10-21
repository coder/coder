import { useEffect, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { getErrorMessage, getErrorStatus } from "#/api/errors";
import { workspaceAgentFindFiles } from "#/api/queries/workspaceFiles";
import {
	Command,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { useDebouncedValue } from "#/hooks/debounce";
import { isMac } from "#/utils/platform";
import {
	useWorkspaceFiles,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../utils/path";

/** Opens a workspace file by fuzzy name search on Cmd+P (macOS) or Ctrl+P. */
export const QuickOpenDialog: React.FC = () => {
	const workspaceFiles = useWorkspaceFiles();
	if (!workspaceFiles) {
		return null;
	}
	return <QuickOpen workspaceFiles={workspaceFiles} />;
};

const QuickOpen: React.FC<{ workspaceFiles: WorkspaceFilesContextValue }> = ({
	workspaceFiles,
}) => {
	const { agentId, repoRoots, openFile } = workspaceFiles;
	const [open, setOpen] = useState(false);
	const [query, setQuery] = useState("");
	const debouncedQuery = useDebouncedValue(query.trim(), 150);
	const roots = repoRoots.length > 0 ? repoRoots : [""];
	const findQuery = useQuery({
		...workspaceAgentFindFiles(agentId, roots, debouncedQuery),
		enabled: open && debouncedQuery !== "",
		placeholderData: keepPreviousData,
	});

	useEffect(() => {
		const handler = (event: KeyboardEvent) => {
			const isModifierPressed = isMac() ? event.metaKey : event.ctrlKey;
			if (
				!isModifierPressed ||
				event.altKey ||
				event.shiftKey ||
				event.key.toLowerCase() !== "p"
			) {
				return;
			}
			event.preventDefault();
			setOpen(true);
		};
		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, []);

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		if (!nextOpen) {
			setQuery("");
		}
	};

	const files =
		debouncedQuery === ""
			? []
			: (findQuery.data?.results.filter((result) => !result.is_dir) ?? []);

	let status: string | undefined;
	if (debouncedQuery === "") {
		status = "Type to search files.";
	} else if (findQuery.isError) {
		status =
			getErrorStatus(findQuery.error) === 404
				? "Update the workspace agent to search files."
				: getErrorMessage(findQuery.error, "Failed to search files.");
	} else if (findQuery.isPending) {
		status = "Searching...";
	} else if (files.length === 0) {
		status = "No matching files.";
	}

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogContent className="max-w-xl gap-0 overflow-hidden p-0">
				<DialogTitle className="sr-only">Open file</DialogTitle>
				<Command shouldFilter={false} label="Search files">
					<CommandInput
						placeholder="Search files by name"
						value={query}
						onValueChange={setQuery}
					/>
					<CommandList>
						{status !== undefined && (
							<div className="py-6 text-center text-sm text-content-secondary">
								{status}
							</div>
						)}
						{!findQuery.isError &&
							files.map((result) => (
								<CommandItem
									key={result.absolute_path}
									value={result.absolute_path}
									onSelect={() => {
										openFile(result.absolute_path);
										handleOpenChange(false);
									}}
									className="flex-col items-start gap-0"
								>
									<span className="font-semibold text-content-primary">
										{getPathBasename(result.path)}
									</span>
									<span className="truncate text-xs font-normal text-content-secondary">
										{roots.length > 1
											? `${getPathBasename(result.root)}/${result.path}`
											: result.path}
									</span>
								</CommandItem>
							))}
					</CommandList>
				</Command>
			</DialogContent>
		</Dialog>
	);
};
