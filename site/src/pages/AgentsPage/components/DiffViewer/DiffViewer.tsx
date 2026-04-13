import type { FC } from "react";
import {
	ManagedDiffViewer,
	type ManagedDiffViewerProps,
} from "#/modules/diffs/ManagedDiffViewer";

interface DiffViewerProps extends ManagedDiffViewerProps {
	diffStyle: DiffStyle;
}

export type DiffStyle = "unified" | "split";
const DIFF_STYLE_KEY = "agents.diff-view-style";

export function loadDiffStyle(): DiffStyle {
	const stored = localStorage.getItem(DIFF_STYLE_KEY);
	if (stored === "split" || stored === "unified") {
		return stored;
	}
	return "unified";
}

export function saveDiffStyle(style: DiffStyle): void {
	localStorage.setItem(DIFF_STYLE_KEY, style);
}

export const DiffViewer: FC<DiffViewerProps> = (props) => {
	return <ManagedDiffViewer {...props} />;
};
