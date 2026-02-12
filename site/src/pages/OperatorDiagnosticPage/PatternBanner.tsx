import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import type { FC } from "react";
import type { DiagnosticPattern } from "./types";

interface PatternBannerProps {
	patterns: DiagnosticPattern[];
}

const severityMap: Record<
	DiagnosticPattern["severity"],
	"info" | "warning" | "error"
> = {
	info: "info",
	warning: "warning",
	critical: "error",
};

export const PatternBanner: FC<PatternBannerProps> = ({ patterns }) => {
	if (patterns.length === 0) return null;

	// Sort: critical first, then warning, then info.
	const sorted = [...patterns].sort((a, b) => {
		const order = { critical: 0, warning: 1, info: 2 };
		return order[a.severity] - order[b.severity];
	});

	return (
		<div className="flex flex-col gap-2">
			{sorted.map((pattern) => (
				<Alert
					key={pattern.id}
					severity={severityMap[pattern.severity]}
					prominent
				>
					<AlertTitle>
						{pattern.title}: {pattern.affected_sessions} of{" "}
						{pattern.total_sessions} sessions
					</AlertTitle>
					<AlertDetail>{pattern.recommendation}</AlertDetail>
				</Alert>
			))}
		</div>
	);
};
