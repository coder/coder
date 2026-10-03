import { useEffect } from "react";
import type { ScaleMode } from "../components/RightPanel/DesktopToolbar";

export function useZoomShortcuts(
	setScaleMode: (mode: ScaleMode) => void,
	enabled = true,
) {
	useEffect(() => {
		if (!enabled) return;
		const handleKeyDown = (e: KeyboardEvent) => {
			const mod = e.ctrlKey || e.metaKey;
			if (mod && e.key === "0") {
				e.preventDefault();
				setScaleMode("fit");
			} else if (mod && e.key === "1") {
				e.preventDefault();
				setScaleMode("native");
			}
		};
		addEventListener("keydown", handleKeyDown);
		return () => removeEventListener("keydown", handleKeyDown);
	}, [enabled, setScaleMode]);
}
