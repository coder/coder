// sortByPriority returns a new array ordered so that items whose id appears in
// `priority` come first, in the order given by `priority`. Items not listed keep
// their original relative order after the prioritized ones. Array.prototype.sort
// is stable, so equal-priority items are never reordered relative to each other.
export function sortByPriority<T extends { id: string }>(
	items: readonly T[],
	priority: readonly string[],
): T[] {
	const indexMap = new Map(priority.map((id, i) => [id, i]));
	const fallback = priority.length;
	return [...items].sort((a, b) => {
		const ai = indexMap.get(a.id) ?? fallback;
		const bi = indexMap.get(b.id) ?? fallback;
		return ai - bi;
	});
}
