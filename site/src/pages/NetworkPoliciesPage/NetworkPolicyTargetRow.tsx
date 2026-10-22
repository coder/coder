import type { NetworkPolicyTarget } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { TableCell, TableRow } from "#/components/Table/Table";

type NetworkPolicyTargetRowProps = {
	target: NetworkPolicyTarget;
	selected: boolean;
	disabled: boolean;
	onSelectedChange: (selected: boolean) => void;
};

/** Displays infrastructure and operation status without owning target selection. */
export const NetworkPolicyTargetRow: React.FC<NetworkPolicyTargetRowProps> = ({
	target,
	selected,
	disabled,
	onSelectedChange,
}) => (
	<TableRow>
		<TableCell>
			<Checkbox
				aria-label={`Select ${target.name}`}
				checked={selected}
				disabled={disabled}
				onCheckedChange={(checked) => onSelectedChange(checked === true)}
			/>
		</TableCell>
		<TableCell>
			<strong>{target.name}</strong>
			<p className="text-sm text-content-secondary">
				{target.cluster} / {target.context} / {target.namespace}
			</p>
			<p className="text-sm text-content-secondary">
				Tags:{" "}
				{Object.entries(target.provisioner_tags)
					.map(([key, value]) => `${key}=${value}`)
					.join(", ") || "None"}
			</p>
		</TableCell>
		<TableCell>
			<Badge>{target.status || "Unknown"}</Badge>
			{target.status === "unknown" && (
				<p className="text-sm text-content-destructive">
					The previous Terraform operation may have changed resources, but its
					outcome could not be confirmed. Further operations are blocked.
					Recovery is not implemented in this POC; inspect the failed run logs
					below for details.
				</p>
			)}
			{(target.status === "pending" || target.status === "running") && (
				<p className="text-sm text-content-secondary">
					An operation is active. Wait for completion before queuing another.
				</p>
			)}
			{target.drifted && <Badge variant="warning">Drift detected</Badge>}
			{target.last_error && (
				<p className="text-sm text-content-destructive">{target.last_error}</p>
			)}
		</TableCell>
		<TableCell>
			<span title={target.desired_revision_id ?? undefined}>
				{target.desired_revision_id?.slice(0, 8) || "None"}
			</span>{" "}
			/{" "}
			<span title={target.applied_revision_id ?? undefined}>
				{target.applied_revision_id?.slice(0, 8) || "Not applied"}
			</span>
		</TableCell>
	</TableRow>
);
