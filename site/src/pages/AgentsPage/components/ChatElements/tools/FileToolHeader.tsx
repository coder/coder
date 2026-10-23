import { ExternalLinkIcon } from "lucide-react";
import type React from "react";
import { useWorkspaceFiles } from "../../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../../utils/path";
import { ToolCall } from "./ToolCall";

/**
 * Opens an absolute path as a file tab. Renders nothing when workspace
 * files are unavailable or the path is relative.
 */
export const OpenFileButton: React.FC<{ path: string }> = ({ path }) => {
	const files = useWorkspaceFiles();
	if (!files || !path.startsWith("/")) {
		return null;
	}
	return (
		<button
			type="button"
			aria-label={`Open ${path}`}
			title={`Open ${path}`}
			className="m-0 inline-flex size-6 shrink-0 cursor-pointer items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary opacity-50 transition-opacity hover:opacity-100 focus-visible:opacity-100"
			onClick={() => files.openFile(path)}
		>
			<ExternalLinkIcon aria-hidden className="size-3" />
		</button>
	);
};

/**
 * Header for tools that act on one file, such as "Read main.go". The label
 * toggles the row like other tools; a separate button at the end opens the
 * file.
 */
export const FileToolHeader: React.FC<{
	iconName: string;
	verb: string;
	path: string;
	running: boolean;
}> = ({ iconName, verb, path, running }) => {
	const filename = getPathBasename(path);
	return (
		<ToolCall.HeaderLayout>
			<ToolCall.Header
				iconName={iconName}
				label={running ? `${verb} ${filename}…` : `${verb} ${filename}`}
			/>
			{!running && (
				<ToolCall.HeaderActions>
					<OpenFileButton path={path} />
				</ToolCall.HeaderActions>
			)}
		</ToolCall.HeaderLayout>
	);
};
