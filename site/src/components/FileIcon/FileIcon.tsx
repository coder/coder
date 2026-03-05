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

// Register the @font-face for the Seti icon font exactly once.
let fontRegistered = false;
function ensureSetiFontRegistered(): void {
	if (fontRegistered || typeof document === "undefined") {
		return;
	}
	fontRegistered = true;
	const style = document.createElement("style");
	style.textContent = `@font-face {
  font-family: "Seti";
  src: url("/seti.woff") format("woff");
  font-weight: normal;
  font-style: normal;
  font-display: block;
}`;
	document.head.appendChild(style);
}

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
	fontFamily: '"Seti"',
	fontSize: 16,
	lineHeight: 1,
	display: "inline-flex",
	alignItems: "center",
	justifyContent: "center",
	width: "1rem",
	height: "1rem",
	userSelect: "none",
	fontStyle: "normal",
	fontWeight: "normal",
	letterSpacing: "normal",
};

export interface FileIconProps {
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
	ensureSetiFontRegistered();

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
