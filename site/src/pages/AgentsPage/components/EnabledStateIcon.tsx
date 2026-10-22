import { CheckIcon, XIcon } from "lucide-react";

type EnabledStateIconProps = {
	enabled: boolean;
};

/** Shows an on or off state to users who cannot change it. */
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
