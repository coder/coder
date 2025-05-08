import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import set from "lodash/set";
import { EditOutlined, RadioButtonCheckedOutlined } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { type FC, useCallback, useMemo } from "react";
import { Link } from "react-router-dom";
import type { FileTree } from "utils/filetree";
import type { TemplateVersionFiles } from "utils/templateVersion";
import { TemplateFileTree } from "./TemplateFileTree";

interface TemplateFilesProps {
	organizationName: string;
	templateName: string;
	versionName: string;
	currentFiles: TemplateVersionFiles;
	/**
	 * Files used to compare with current files
	 */
	baseFiles?: TemplateVersionFiles;
}

export const TemplateFiles: FC<TemplateFilesProps> = ({
	organizationName,
	templateName,
	versionName,
	currentFiles,
	baseFiles,
}) => {
	const getLink = useLinks();
	const theme = useTheme();

	const fileInfo = useCallback(
		(filename: string) => {
			const value = currentFiles[filename].trim();
			const previousValue = baseFiles ? baseFiles[filename]?.trim() : undefined;
			const hasDiff = previousValue && value !== previousValue;

			return {
				value,
				previousValue,
				hasDiff,
			};
		},
		[baseFiles, currentFiles],
	);

	const fileTree: FileTree = useMemo(() => {
		const tree: FileTree = {};
		for (const filename of Object.keys(currentFiles)) {
			const info = fileInfo(filename);
			set(tree, filename.split("/"), info.value);
		}
		return tree;
	}, [fileInfo, currentFiles]);

	const versionLink = `${getLink(
		linkToTemplate(organizationName, templateName),
	)}/versions/${versionName}`;

	return (
		<div>
			<div css={{ display: "flex", alignItems: "flex-start", gap: 32 }}>
				<div css={styles.sidebar}>
					<TemplateFileTree
						fileTree={fileTree}
						onSelect={(path: string) => {
							window.location.hash = path;
						}}
						Label={({ path, filename, isFolder }) => {
							if (isFolder) {
								return <>{filename}</>;
							}

							const hasDiff = fileInfo(path).hasDiff;
							return (
								<span
									css={hasDiff && { color: theme.roles.warning.fill.outline }}
								>
									{filename}
								</span>
							);
						}}
					/>
				</div>

				<div css={styles.files} data-testid="template-files-content">
					{Object.keys(currentFiles)
						.sort((a, b) => a.localeCompare(b))
						.map((filename) => {
							const info = fileInfo(filename);

							return (
								<div key={filename} css={styles.filePanel} id={filename}>
									<header css={styles.fileHeader}>
										{filename}
										{info.hasDiff && (
											<RadioButtonCheckedOutlined
												css={{
													width: 14,
													height: 14,
													color: theme.roles.warning.fill.outline,
												}}
											/>
										)}

										<div css={{ marginLeft: "auto" }}>
											<Link
												to={`${versionLink}/edit?path=${filename}`}
												css={{
													display: "flex",
													gap: 4,
													alignItems: "center",
													fontSize: 14,
													color: theme.palette.text.secondary,
													textDecoration: "none",

													"&:hover": {
														color: theme.palette.text.primary,
													},
												}}
											>
												<EditOutlined css={{ fontSize: "inherit" }} />
												Edit
											</Link>
										</div>
									</header>
									<SyntaxHighlighter
										language={getLanguage(filename)}
										value={info.value}
										compareWith={info.previousValue}
										editorProps={{
											// 18 is the editor line height
											height: Math.min(numberOfLines(info.value) * 18, 560),
											onMount: (editor) => {
												editor.updateOptions({
													scrollBeyondLastLine: false,
												});
											},
										}}
									/>
								</div>
							);
						})}
				</div>
			</div>
		</div>
	);
};

const languageByExtension: Record<string, string> = {
	tf: "hcl",
	hcl: "hcl",
	md: "markdown",
	mkd: "markdown",
	sh: "shell",
	tpl: "tpl",
	protobuf: "protobuf",
	nix: "dockerfile",
	json: "json",
};

const getLanguage = (filename: string) => {
	// Dockerfile can be like images/Dockerfile or Dockerfile.java
	if (filename.includes("Dockerfile")) {
		return "dockerfile";
	}
	const extension = filename.split(".").pop();
	return languageByExtension[extension ?? ""];
};

const numberOfLines = (content: string) => {
	return content.split("\n").length;
};

const styles = {
	sidebar: (theme) => ({
		width: 240,
		flexShrink: 0,
		borderRadius: 8,
		overflow: "auto",
		border: `1px solid ${theme.palette.divider}`,
		padding: "4px 0",
		position: "sticky",
		top: 32,
	}),

	files: {
		display: "flex",
		flexDirection: "column",
		gap: 16,
		flex: 1,
	},

	filePanel: (theme) => ({
		borderRadius: 8,
		border: `1px solid ${theme.palette.divider}`,
		overflow: "hidden",
	}),

	fileHeader: (theme) => ({
		padding: "8px 16px",
		borderBottom: `1px solid ${theme.palette.divider}`,
		fontSize: 13,
		fontWeight: 500,
		display: "flex",
		gap: 8,
		alignItems: "center",
	}),
} satisfies Record<string, Interpolation<Theme>>;
