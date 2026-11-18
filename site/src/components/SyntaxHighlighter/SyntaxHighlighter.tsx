import type {
	BaseCodeOptions,
	FileContents,
	SupportedLanguages,
} from "@pierre/diffs";
import { File, MultiFileDiff } from "@pierre/diffs/react";
import { MONOSPACE_FONT_FAMILY } from "#/theme/constants";
import { useTheme } from "#/theme/context";
import { DIFFS_GIT_COLOR_CSS, DIFFS_GIT_COLOR_STYLE } from "./diffsGitColors";
import { coderShikiThemes } from "./shikiTheme";

type SyntaxHighlighterProps = {
	/** Used to infer the language, unless `language` is set. */
	filename: string;
	value: string;
	/** Previous contents of the file. A diff is shown when they differ. */
	compareWith?: string;
	language?: SupportedLanguages;
};

const style = {
	"--diffs-font-family": MONOSPACE_FONT_FAMILY,
	"--diffs-font-size": "13px",
	...DIFFS_GIT_COLOR_STYLE,
} satisfies React.CSSProperties;

export const SyntaxHighlighter: React.FC<SyntaxHighlighterProps> = ({
	filename,
	value,
	compareWith,
	language,
}) => {
	const theme = useTheme();
	// Shared by both `File` and `MultiFileDiff`, whose options extend this type.
	const options: BaseCodeOptions = {
		theme: coderShikiThemes,
		themeType: theme.mode,
		disableFileHeader: true,
		overflow: "scroll",
		unsafeCSS: DIFFS_GIT_COLOR_CSS,
	};

	const file: FileContents = {
		name: filename,
		contents: value,
		lang: language,
	};

	return (
		<div data-pixel="ignore" className="max-h-140 overflow-auto">
			{compareWith && value !== compareWith ? (
				<MultiFileDiff
					oldFile={{ ...file, contents: compareWith }}
					newFile={file}
					options={{ ...options, diffStyle: "split" }}
					style={style}
				/>
			) : (
				<File file={file} options={options} style={style} />
			)}
		</div>
	);
};
