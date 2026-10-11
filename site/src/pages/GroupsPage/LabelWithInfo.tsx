import { StatusIconTooltip } from "./StatusIconTooltip";

/** A label followed by an info icon whose tooltip explains it. */
export const LabelWithInfo: React.FC<{
	label: React.ReactNode;
	message: React.ReactNode;
}> = ({ label, message }) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<StatusIconTooltip message={message} />
	</span>
);
