import { DockerIcon } from "components/Icons/DockerIcon";
import {
	BracesIcon,
	FileCodeIcon,
	FileIcon,
	FolderIcon,
	TerminalIcon,
} from "lucide-react";
import type { ComponentProps, ElementType, FC } from "react";

const FileTypeTerraform: FC<ComponentProps<"svg">> = (props) => (
	<svg
		xmlns="http://www.w3.org/2000/svg"
		viewBox="0 0 32 32"
		fill="#813cf3"
		{...props}
	>
		<title>file_type_terraform</title>
		<polygon points="12.042 6.858 20.071 11.448 20.071 20.462 12.042 15.868 12.042 6.858 12.042 6.858" />
		<polygon points="20.5 20.415 28.459 15.84 28.459 6.887 20.5 11.429 20.5 20.415 20.5 20.415" />
		<polygon points="3.541 11.01 11.571 15.599 11.571 6.59 3.541 2 3.541 11.01 3.541 11.01" />
		<polygon points="12.042 25.41 20.071 30 20.071 20.957 12.042 16.368 12.042 25.41 12.042 25.41" />
	</svg>
);

const FileTypeMarkdown: FC<ComponentProps<"svg">> = (props) => (
	<svg
		xmlns="http://www.w3.org/2000/svg"
		viewBox="0 0 32 32"
		fill="#755838"
		role="img"
		aria-label="Markdown icon"
		{...props}
	>
		<rect
			x="2.5"
			y="7.955"
			width="27"
			height="16.091"
			style={{
				fill: "none",
				stroke: "#755838",
			}}
		/>
		<polygon points="5.909 20.636 5.909 11.364 8.636 11.364 11.364 14.773 14.091 11.364 16.818 11.364 16.818 20.636 14.091 20.636 14.091 15.318 11.364 18.727 8.636 15.318 8.636 20.636 5.909 20.636" />
		<polygon points="22.955 20.636 18.864 16.136 21.591 16.136 21.591 11.364 24.318 11.364 24.318 16.136 27.045 16.136 22.955 20.636" />
	</svg>
);

export const getTemplateFileIcon = (
	filename: string,
	isFolder: boolean,
): ElementType => {
	if (isFolder) {
		return FolderIcon;
	}
	if (filename.endsWith(".tf")) {
		return FileTypeTerraform;
	}
	if (filename.endsWith(".md")) {
		return FileTypeMarkdown;
	}
	if (filename.endsWith("Dockerfile")) {
		return DockerIcon;
	}
	if (filename.endsWith(".sh")) {
		return TerminalIcon;
	}
	if (filename.endsWith(".json")) {
		return BracesIcon;
	}
	if (filename.endsWith(".yaml") || filename.endsWith(".yml")) {
		return FileCodeIcon;
	}
	// Default icon for files without a specific icon.
	return FileIcon;
};
