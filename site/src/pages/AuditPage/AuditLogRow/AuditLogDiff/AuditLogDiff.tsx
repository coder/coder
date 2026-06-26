import type { FC } from "react";
import type { AuditDiff } from "#/api/typesGenerated";
import { formatAuditDiffValue } from "./auditUtils";

interface AuditLogDiffProps {
	diff: AuditDiff;
}

export const AuditLogDiff: FC<AuditLogDiffProps> = ({ diff }) => {
	const diffEntries = Object.entries(diff);

	return (
		<div className="relative z-[2] flex items-start border-t border-border font-mono text-sm">
			<div className="flex-1 self-stretch bg-red-950 pb-5 pr-4 pt-4 leading-[160%] text-red-50 [overflow-wrap:anywhere]">
				{diffEntries.map(([attrName, valueDiff], index) => (
					<div key={attrName} className="flex items-baseline">
						<div className="w-12 shrink-0 text-right opacity-50">
							{index + 1}
						</div>
						<div className="w-8 shrink-0 text-center text-base">-</div>
						<div>
							{attrName}:{" "}
							<span className="rounded p-px bg-red-800">
								{valueDiff.secret
									? "••••••••"
									: formatAuditDiffValue(valueDiff.old)}
							</span>
						</div>
					</div>
				))}
			</div>
			<div className="flex-1 self-stretch bg-green-950 pb-5 pr-4 pt-4 leading-[160%] text-green-50 [overflow-wrap:anywhere]">
				{diffEntries.map(([attrName, valueDiff], index) => (
					<div key={attrName} className="flex items-baseline">
						<div className="w-12 shrink-0 text-right opacity-50">
							{index + 1}
						</div>
						<div className="w-8 shrink-0 text-center text-base">+</div>
						<div>
							{attrName}:{" "}
							<span className="rounded p-px bg-green-800">
								{valueDiff.secret
									? "••••••••"
									: formatAuditDiffValue(valueDiff.new)}
							</span>
						</div>
					</div>
				))}
			</div>
		</div>
	);
};
