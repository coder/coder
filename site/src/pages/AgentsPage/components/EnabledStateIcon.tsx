import { CheckIcon, XIcon } from "lucide-react";

type EnabledStateIconProps = {
	enabled: boolean;
};

/** A non-interactive check or x, announced as "Enabled" or "Disabled". */
export const EnabledStateIcon: React.FC<EnabledStateIconProps> = ({
	enabled,
}) =>
	enabled ? (
		<CheckIcon
			role="img"
			aria-label="Enabled"
			className="size-icon-sm text-content-success"
		/>
	) : (
		<XIcon
			role="img"
			aria-label="Disabled"
			className="size-icon-sm text-content-secondary"
		/>
	);
