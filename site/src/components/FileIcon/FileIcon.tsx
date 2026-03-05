import { type FC, useMemo } from "react";
import setiIconTheme from "./seti-icon-theme.json";

interface SetiIconDefinition {
	fontCharacter?: string;
	fontColor?: string;
}

interface FileIconGlyph {
	character: string;
	color?: string;
}

const setiIconDefinitions: Record<string, SetiIconDefinition> =
	setiIconTheme.iconDefinitions;
const setiDefaultIconId = setiIconTheme.file;
const setiFileNames = setiIconTheme.fileNames as Record<string, string>;
const setiFileExtensions = setiIconTheme.fileExtensions as Record<
	string,
	string
>;
const setiLanguageIds = setiIconTheme.languageIds as Record<string, string>;

const setiDefaultIconDefinition: SetiIconDefinition = setiIconDefinitions[
	setiDefaultIconId
] ?? {
	fontCharacter: "\\E023",
};

const decodeFontCharacter = (encoded?: string): string => {
	if (!encoded) {
		return "";
	}
	if (!encoded.startsWith("\\")) {
		return encoded;
	}

	const hex = encoded.slice(1);
	const codePoint = Number.parseInt(hex, 16);
	if (Number.isNaN(codePoint)) {
		return "";
	}

	return String.fromCodePoint(codePoint);
};

const collectExtensionCandidates = (fileName: string): string[] => {
	const parts = fileName.split(".");
	if (parts.length <= 1) {
		return [];
	}

	const candidates: string[] = [];
	for (let i = 1; i < parts.length; i++) {
		const candidate = parts.slice(i).join(".");
		if (candidate) {
			candidates.push(candidate);
		}
	}

	return candidates;
};

/**
 * Maps common file extensions to VS Code language identifiers
 * when the extension alone doesn't match a key in the Seti
 * theme's `languageIds` map.
 */
const extToLanguageId: Record<string, string> = {
	js: "javascript",
	jsx: "javascriptreact",
	mjs: "javascript",
	cjs: "javascript",
	py: "python",
	rb: "ruby",
	rs: "rust",
	md: "markdown",
	mdx: "markdown",
	yml: "yaml",
	sh: "shellscript",
	bash: "shellscript",
	zsh: "shellscript",
	fish: "shellscript",
	ps1: "powershell",
	cs: "csharp",
	fs: "fsharp",
	kt: "kotlin",
	kts: "kotlin",
	swift: "swift",
	pl: "perl",
	php: "php",
	ex: "elixir",
	exs: "elixir",
	erl: "erlang",
	hrl: "erlang",
	hs: "haskell",
	lua: "lua",
	vim: "viml",
	clj: "clojure",
	cljs: "clojure",
	cljc: "clojure",
	jl: "julia",
	r: "r",
	ml: "ocaml",
	mli: "ocaml",
	nim: "nim",
	nix: "nix",
	tf: "terraform",
	tfvars: "terraform",
	hcl: "terraform",
	sql: "sql",
	gql: "graphql",
	graphql: "graphql",
	proto: "proto3",
	svg: "xml",
	xml: "xml",
	html: "html",
	htm: "html",
	css: "css",
	scss: "scss",
	sass: "sass",
	less: "less",
	styl: "stylus",
	vue: "vue",
	svelte: "svelte",
	java: "java",
	scala: "scala",
	groovy: "groovy",
	dart: "dart",
	elm: "elm",
	tpx: "typoscript",
};

const resolveSetiIconId = (fileName: string): string | undefined => {
	const direct = setiFileNames[fileName];
	if (direct) {
		return direct;
	}

	const lowerName = fileName.toLowerCase();
	if (lowerName !== fileName) {
		const lowerDirect = setiFileNames[lowerName];
		if (lowerDirect) {
			return lowerDirect;
		}
	}

	if (fileName.startsWith(".") && fileName.length > 1) {
		const withoutDot = fileName.slice(1);
		const withoutDotMatch = setiFileNames[withoutDot];
		if (withoutDotMatch) {
			return withoutDotMatch;
		}
	}

	const extensionCandidates = collectExtensionCandidates(fileName);

	for (const candidate of extensionCandidates) {
		const extMatch = setiFileExtensions[candidate];
		if (extMatch) {
			return extMatch;
		}

		const lowerCandidate = candidate.toLowerCase();
		if (lowerCandidate !== candidate) {
			const lowerExtMatch = setiFileExtensions[lowerCandidate];
			if (lowerExtMatch) {
				return lowerExtMatch;
			}
		}
	}

	const languageMatch = setiLanguageIds[lowerName];
	if (languageMatch) {
		return languageMatch;
	}

	for (const candidate of extensionCandidates) {
		const languageIdMatch = setiLanguageIds[candidate];
		if (languageIdMatch) {
			return languageIdMatch;
		}

		const lowerCandidate = candidate.toLowerCase();
		if (lowerCandidate !== candidate) {
			const lowerLanguageIdMatch = setiLanguageIds[lowerCandidate];
			if (lowerLanguageIdMatch) {
				return lowerLanguageIdMatch;
			}
		}
	}

	// Try mapping the extension to a known language identifier.
	for (const candidate of extensionCandidates) {
		const langId = extToLanguageId[candidate.toLowerCase()];
		if (langId) {
			const langMatch = setiLanguageIds[langId];
			if (langMatch) {
				return langMatch;
			}
		}
	}

	if (fileName.startsWith(".") && fileName.length > 1) {
		const trimmedLower = fileName.slice(1).toLowerCase();
		const trimmedLanguageMatch = setiLanguageIds[trimmedLower];
		if (trimmedLanguageMatch) {
			return trimmedLanguageMatch;
		}
	}

	return undefined;
};

const getSetiIconForFile = (fileName: string): FileIconGlyph => {
	if (!fileName) {
		return {
			character:
				decodeFontCharacter(setiDefaultIconDefinition.fontCharacter) || " ",
			color: setiDefaultIconDefinition.fontColor,
		};
	}

	const iconId = resolveSetiIconId(fileName);
	const iconDefinition = iconId ? setiIconDefinitions[iconId] : undefined;

	return {
		character:
			decodeFontCharacter(iconDefinition?.fontCharacter) ||
			decodeFontCharacter(setiDefaultIconDefinition.fontCharacter) ||
			" ",
		color: iconDefinition?.fontColor ?? setiDefaultIconDefinition.fontColor,
	};
};

const BASE_ICON_STYLE: React.CSSProperties = {
	fontFamily:
		'"seti", "Geist Mono Variable", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
	fontSize: 22,
	lineHeight: 1,
	display: "inline-flex",
	alignItems: "center",
	justifyContent: "center",
	minWidth: "1.375rem",
	height: "1.375rem",
	userSelect: "none",
	fontStyle: "normal",
	fontWeight: "normal",
	letterSpacing: "normal",
};

interface FileIconProps {
	fileName?: string | null;
	filePath?: string | null;
	className?: string;
	style?: React.CSSProperties;
}

export const FileIcon: FC<FileIconProps> = ({
	fileName,
	filePath,
	className,
	style,
}) => {
	const targetName =
		fileName ?? (filePath ? (filePath.split("/").pop() ?? "") : "");

	const icon = useMemo(
		() => getSetiIconForFile(targetName ?? ""),
		[targetName],
	);

	if (!icon.character.trim()) {
		return null;
	}

	return (
		<span
			aria-hidden="true"
			className={className ?? undefined}
			style={{ ...BASE_ICON_STYLE, color: icon.color, ...style }}
		>
			{icon.character}
		</span>
	);
};
