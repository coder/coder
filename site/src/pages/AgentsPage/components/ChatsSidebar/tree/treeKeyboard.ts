export type TreeKeyboardRow = {
	readonly id: string;
	readonly level: number;
	readonly parentId?: string;
	readonly hasChildren: boolean;
	readonly isExpanded: boolean;
	/** Expanded by a filter rather than the user; Left must not collapse it. */
	readonly isForceExpanded?: boolean;
	readonly label: string;
};

type TreeKeyboardResult = {
	readonly handled: boolean;
	readonly focusId?: string;
	readonly toggle?: { readonly id: string; readonly expanded: boolean };
	/** Ids to expand when `*` is pressed: collapsed siblings with children. */
	readonly expandIds?: readonly string[];
	/** The type-ahead buffer after this key, empty when the key was not a character. */
	readonly typeahead: string;
};

const notHandled = (typeahead = ""): TreeKeyboardResult => ({
	handled: false,
	typeahead,
});

const findTypeaheadMatch = (
	rows: readonly TreeKeyboardRow[],
	startIndex: number,
	prefix: string,
): string | undefined => {
	for (let offset = 0; offset < rows.length; offset++) {
		const row = rows[(startIndex + offset) % rows.length];
		if (row.label.toLowerCase().startsWith(prefix)) {
			return row.id;
		}
	}
	return undefined;
};

/**
 * Picks the row to focus once `removedId` and its subtree leave the tree:
 * the next visible row outside the subtree, else the previous row, else
 * nothing (the tree is empty).
 */
export const focusTargetAfterRemoval = (
	rows: readonly TreeKeyboardRow[],
	removedId: string,
): string | undefined => {
	const index = rows.findIndex((row) => row.id === removedId);
	if (index === -1) {
		return undefined;
	}
	const removedLevel = rows[index].level;
	for (let cursor = index + 1; cursor < rows.length; cursor++) {
		if (rows[cursor].level <= removedLevel) {
			return rows[cursor].id;
		}
	}
	return index > 0 ? rows[index - 1].id : undefined;
};

/**
 * WAI-ARIA tree keyboard pattern over the visible rows. Pure: the caller
 * owns focus, expansion state, and the type-ahead buffer (which it resets
 * after a pause). Right and Left follow the LTR convention. Enter is not
 * handled here: activation is the treeitem's own click or key handler.
 */
export const treeKeyboardReducer = (
	rows: readonly TreeKeyboardRow[],
	focusedId: string | undefined,
	key: string,
	typeaheadBuffer: string,
): TreeKeyboardResult => {
	if (rows.length === 0) {
		return notHandled();
	}
	const index = rows.findIndex((row) => row.id === focusedId);
	const current = index === -1 ? undefined : rows[index];

	switch (key) {
		case "ArrowDown": {
			const next =
				index === -1 ? rows[0] : rows[Math.min(index + 1, rows.length - 1)];
			return { handled: true, focusId: next.id, typeahead: "" };
		}
		case "ArrowUp": {
			const previous = index === -1 ? rows[0] : rows[Math.max(index - 1, 0)];
			return { handled: true, focusId: previous.id, typeahead: "" };
		}
		case "Home":
			return { handled: true, focusId: rows[0].id, typeahead: "" };
		case "End":
			return {
				handled: true,
				focusId: rows[rows.length - 1].id,
				typeahead: "",
			};
		case "ArrowRight": {
			if (!current?.hasChildren) {
				return { handled: Boolean(current), typeahead: "" };
			}
			if (!current.isExpanded) {
				return {
					handled: true,
					toggle: { id: current.id, expanded: true },
					typeahead: "",
				};
			}
			const firstChild = rows[index + 1];
			return {
				handled: true,
				focusId:
					firstChild?.parentId === current.id ? firstChild.id : undefined,
				typeahead: "",
			};
		}
		case "ArrowLeft": {
			if (!current) {
				return notHandled();
			}
			if (
				current.hasChildren &&
				current.isExpanded &&
				!current.isForceExpanded
			) {
				return {
					handled: true,
					toggle: { id: current.id, expanded: false },
					typeahead: "",
				};
			}
			return {
				handled: true,
				focusId: current.parentId,
				typeahead: "",
			};
		}
		case "*": {
			if (!current) {
				return notHandled();
			}
			const expandIds = rows
				.filter(
					(row) =>
						row.parentId === current.parentId &&
						row.level === current.level &&
						row.hasChildren &&
						!row.isExpanded,
				)
				.map((row) => row.id);
			return { handled: true, expandIds, typeahead: "" };
		}
		default: {
			if (key.length !== 1 || key === " ") {
				return notHandled();
			}
			const character = key.toLowerCase();
			const buffer = `${typeaheadBuffer}${character}`;
			// A repeated single character cycles through rows starting with
			// it; a longer buffer refines the match from the focused row.
			const cycling =
				buffer.length > 1 && buffer.split("").every((c) => c === character);
			const prefix = cycling ? character : buffer;
			const startIndex =
				index === -1 || cycling || buffer.length === 1 ? index + 1 : index;
			const match = findTypeaheadMatch(rows, startIndex, prefix);
			return {
				handled: true,
				focusId: match,
				typeahead: prefix,
			};
		}
	}
};
