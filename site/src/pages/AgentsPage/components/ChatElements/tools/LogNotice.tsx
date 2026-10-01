import { InfoIcon, LoaderIcon } from "lucide-react";

type LogNoticeProps = {
	icon?: "loading" | "info";
	children: React.ReactNode;
};

/** Status line inside the workspace log box. */
export const LogNotice: React.FC<LogNoticeProps> = ({ icon, children }) => (
	<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
		{icon === "loading" && (
			<LoaderIcon className="size-3 animate-spin motion-reduce:animate-none" />
		)}
		{icon === "info" && <InfoIcon className="size-3 shrink-0" />}
		<span>{children}</span>
	</div>
);
