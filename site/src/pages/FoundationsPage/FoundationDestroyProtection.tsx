import { useId } from "react";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Label } from "#/components/Label/Label";

type FoundationDestroyProtectionProps = {
	checked: boolean;
	disabled: boolean;
	onChange: (checked: boolean) => void;
};

/** Configures the Destroy guard without implying protection from ordinary Apply. */
export const FoundationDestroyProtection: React.FC<
	FoundationDestroyProtectionProps
> = ({ checked, disabled, onChange }) => {
	const id = useId();
	return (
		<div className="space-y-3">
			<div className="flex items-center gap-2">
				<Checkbox
					id={id}
					checked={checked}
					disabled={disabled}
					onCheckedChange={(value) => onChange(value === true)}
				/>
				<Label htmlFor={id}>Prevent Destroy while workspaces exist</Label>
			</div>
			<p className="text-sm text-content-secondary">
				Enabled by default. This guard applies only to Destroy. Ordinary Apply
				can still delete infrastructure used by workspaces.
			</p>
			{!checked && (
				<p className="text-sm text-content-destructive" role="status">
					Disabling this protection allows Destroy with running or stopped
					consumers and can break those workspaces.
				</p>
			)}
		</div>
	);
};
