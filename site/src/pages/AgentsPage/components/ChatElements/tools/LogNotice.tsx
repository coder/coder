import { LoaderIcon, TriangleAlertIcon } from "lucide-react";

type LogNoticeProps = {
	icon?: "loading" | "warning";
	children: React.ReactNode;
};

/** Status line shown in place of a build or agent log box. */
export const LogNotice: React.FC<LogNoticeProps> = ({ icon, children }) => (
	<div className="flex items-center gap-2 py-3 px-4 text-xs text-content-secondary">
		{icon === "loading" && (
			<LoaderIcon className="size-3 animate-spin motion-reduce:animate-none" />
		)}
		{icon === "warning" && <TriangleAlertIcon className="size-3" />}
		<span>{children}</span>
	</div>
);
