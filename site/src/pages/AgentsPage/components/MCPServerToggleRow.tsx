import type { FC, ReactNode } from "react";
import { Switch } from "#/components/Switch/Switch";

interface MCPServerToggleRowProps {
	icon: ReactNode;
	label: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
	/** Force On servers stay on; the switch renders disabled. */
	locked?: boolean;
	disabled?: boolean;
	/** Replaces the switch while the server needs setup, such as OAuth. */
	action?: ReactNode;
	/** Rendered between the label and the switch. */
	children?: ReactNode;
}

/**
 * One per-chat MCP server row in the chat input `+` menu. Coder-managed
 * and workspace `.mcp.json` servers both render through this row.
 */
export const MCPServerToggleRow: FC<MCPServerToggleRowProps> = ({
	icon,
	label,
	checked,
	onCheckedChange,
	locked,
	disabled,
	action,
	children,
}) => (
	<div className="flex items-center gap-1.5 px-1 py-1.5">
		{icon}
		<span className="min-w-0 flex-1 truncate text-xs text-content-secondary">
			{label}
		</span>
		{children}
		{action ?? (
			<Switch
				size="sm"
				checked={checked}
				onCheckedChange={onCheckedChange}
				disabled={disabled || locked}
				aria-label={`${checked ? "Disable" : "Enable"} ${label}`}
			/>
		)}
	</div>
);
