import type React from "react";
import { useWorkspaceFiles } from "../../../context/WorkspaceFilesContext";
import { getPathBasename } from "../../../utils/path";
import { ToolCall } from "./ToolCall";

/**
 * Header for tools that act on one file, such as "Read main.go". When
 * workspace files are available and the path is absolute, the file name is
 * a separate button that opens the file without toggling the row.
 */
export const FileToolHeader: React.FC<{
	iconName: string;
	verb: string;
	path: string;
	running: boolean;
}> = ({ iconName, verb, path, running }) => {
	const files = useWorkspaceFiles();
	const filename = getPathBasename(path);
	if (!files || running || !path.startsWith("/")) {
		return (
			<ToolCall.Header
				iconName={iconName}
				label={running ? `${verb} ${filename}…` : `${verb} ${filename}`}
			/>
		);
	}

	return (
		<ToolCall.HeaderLayout className="text-content-secondary">
			<ToolCall.HeaderButton className="shrink-0">
				<ToolCall.LeadingIcon name={iconName} />
				<ToolCall.Label>
					{verb} <span className="sr-only">{filename}</span>
				</ToolCall.Label>
			</ToolCall.HeaderButton>
			<button
				type="button"
				aria-label={`Open ${path}`}
				title={path}
				className="m-0 min-w-0 cursor-pointer truncate border-0 bg-transparent p-0 text-left font-[inherit] text-[13px] leading-6 text-inherit underline decoration-dotted underline-offset-2 hover:text-content-link"
				onClick={() => files.openFile(path)}
			>
				{filename}
			</button>
			<ToolCall.Status />
			<ToolCall.State>
				{({ collapsible, onToggle }) =>
					collapsible && (
						// The labelled toggle is the header button; this one only
						// widens the mouse target to the chevron.
						<button
							type="button"
							tabIndex={-1}
							aria-hidden
							className="m-0 flex cursor-pointer border-0 bg-transparent p-0 text-inherit hover:text-content-primary"
							onClick={onToggle}
						>
							<ToolCall.Chevron />
						</button>
					)
				}
			</ToolCall.State>
		</ToolCall.HeaderLayout>
	);
};
