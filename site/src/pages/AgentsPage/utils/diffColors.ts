import type { ChangeTypes } from "@pierre/diffs";

/** Maps a diff change type to a Tailwind text-color class. */
export function changeColor(type?: ChangeTypes): string | undefined {
	switch (type) {
		case "new":
			return "text-git-added";
		case "deleted":
			return "text-git-deleted";
		case "rename-pure":
		case "rename-changed":
			return "text-git-modified";
		case "change":
			return "text-git-modified";
		default:
			return undefined;
	}
}

/** Short letter shown after the filename, matching VS Code style. */
export function changeLabel(type: ChangeTypes): string {
	switch (type) {
		case "new":
			return "A";
		case "deleted":
			return "D";
		case "rename-pure":
		case "rename-changed":
			return "R";
		case "change":
			return "M";
		default:
			return "";
	}
}
