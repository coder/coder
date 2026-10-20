import { useState } from "react";
import {
	countFittingColumns,
	parseSessionColumnSettings,
	type SessionColumnSettings,
	serializeSessionColumnSettings,
} from "./sessionColumns";

/** Local storage key for a user's column settings. */
export const sessionColumnsStorageKey = (userId: string) =>
	`aiGateway.sessions.columnSettings.${userId}`;

/** Tracks saved column settings and which shown columns fit the `tableRef` element. */
export const useSessionColumns = (userId: string) => {
	const storageKey = sessionColumnsStorageKey(userId);
	const [settings, setSettings] = useState(() =>
		parseSessionColumnSettings(localStorage.getItem(storageKey)),
	);
	const [tableWidth, setTableWidth] = useState<number>();

	const updateSettings = (next: SessionColumnSettings) => {
		setSettings(next);
		localStorage.setItem(storageKey, serializeSessionColumnSettings(next));
	};

	const tableRef = (element: HTMLElement | null) => {
		if (!element) {
			return;
		}
		const observer = new ResizeObserver(([entry]) => {
			setTableWidth(entry.contentRect.width);
		});
		observer.observe(element);
		return () => observer.disconnect();
	};

	// Until the first measurement, and in environments without layout, show
	// every column rather than collapsing the table to one.
	const fittingCount =
		tableWidth === undefined || tableWidth === 0
			? settings.shown.length
			: countFittingColumns(settings.shown, tableWidth);

	return {
		settings,
		updateSettings,
		tableRef,
		fittingCount,
		visibleColumns: settings.shown.slice(0, fittingCount),
	};
};
