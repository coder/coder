import type { FC } from "react";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	type SidebarChatLayout,
	useSidebarChatLayout,
} from "../hooks/useSidebarChatLayout";

const layoutOptions: { value: SidebarChatLayout; label: string }[] = [
	{ value: "two_line", label: "Two lines" },
	{ value: "one_line", label: "One line" },
];

export const SidebarChatLayoutSettings: FC = () => {
	const [layout, setLayout] = useSidebarChatLayout();

	return (
		<div className="flex flex-col gap-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Sidebar chat layout
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p className="m-0 flex-1 text-xs text-content-secondary">
					How chats appear in the sidebar. 'One line' shows the chat status,
					title, pull request status, and shared indicator. 'Two lines' adds the
					latest summary, pull request changes, and time of last activity.
				</p>
				<Select
					value={layout}
					onValueChange={(value: string) => {
						const selected = layoutOptions.find((opt) => opt.value === value);
						if (selected) {
							setLayout(selected.value);
						}
					}}
				>
					<SelectTrigger
						className="w-44 shrink-0"
						aria-label="Sidebar chat layout"
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{layoutOptions.map((opt) => (
							<SelectItem key={opt.value} value={opt.value}>
								{opt.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
		</div>
	);
};
