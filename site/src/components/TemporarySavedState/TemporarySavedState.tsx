import { CheckIcon } from "lucide-react";
import type { FC } from "react";
import { useEffect, useRef, useState } from "react";

export const useTemporarySavedState = (
	durationMs = 2500,
): {
	isSavedVisible: boolean;
	showSavedState: () => void;
} => {
	const [isSavedVisible, setIsSavedVisible] = useState(false);
	const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	useEffect(() => {
		return () => {
			if (timeoutRef.current) {
				clearTimeout(timeoutRef.current);
			}
		};
	}, []);

	const showSavedState = () => {
		if (timeoutRef.current) {
			clearTimeout(timeoutRef.current);
		}
		setIsSavedVisible(true);
		timeoutRef.current = setTimeout(() => {
			setIsSavedVisible(false);
			timeoutRef.current = null;
		}, durationMs);
	};

	return { isSavedVisible, showSavedState };
};

export const TemporarySavedState: FC = () => (
	<div
		aria-live="polite"
		className="inline-flex min-w-8 min-h-6 shrink-0 items-center justify-center gap-1 rounded-md border border-border-success bg-surface-success px-2 font-sans text-2xs font-medium whitespace-nowrap text-content-success"
	>
		<CheckIcon className="size-3.5" />
		<span>Saved</span>
	</div>
);
