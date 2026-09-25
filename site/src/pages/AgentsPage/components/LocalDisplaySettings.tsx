import type { FC } from "react";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	type AgentFontSize,
	useAgentFontSize,
} from "../hooks/useAgentFontSize";
import {
	type SidebarChatLayout,
	useSidebarChatLayout,
} from "../hooks/useSidebarChatLayout";

type Option<T extends string> = { value: T; label: string };

type LocalPreferenceSelectProps<T extends string> = {
	title: string;
	description: string;
	value: T;
	options: readonly Option<T>[];
	onChange: (value: T) => void;
};

// A titled select for display preferences stored in the browser.
const LocalPreferenceSelect = <T extends string>({
	title,
	description,
	value,
	options,
	onChange,
}: LocalPreferenceSelectProps<T>) => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">{title}</h3>
		<div className="flex items-center justify-between gap-4">
			<p className="m-0 flex-1 text-xs text-content-secondary">{description}</p>
			<Select
				value={value}
				onValueChange={(next: string) => {
					const selected = options.find((opt) => opt.value === next);
					if (selected) {
						onChange(selected.value);
					}
				}}
			>
				<SelectTrigger className="w-44 shrink-0" aria-label={title}>
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					{options.map((opt) => (
						<SelectItem key={opt.value} value={opt.value}>
							{opt.label}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	</div>
);

const layoutOptions: readonly Option<SidebarChatLayout>[] = [
	{ value: "two_line", label: "Two lines" },
	{ value: "one_line", label: "One line" },
];

export const SidebarChatLayoutSettings: FC = () => {
	const [layout, setLayout] = useSidebarChatLayout();
	return (
		<LocalPreferenceSelect
			title="Sidebar chat layout"
			description="How chats appear in the sidebar. 'One line' shows the chat status, title, pull request status, and shared indicator. 'Two lines' adds the latest summary, pull request changes, and time of last activity."
			value={layout}
			options={layoutOptions}
			onChange={setLayout}
		/>
	);
};

const fontSizeOptions: readonly Option<AgentFontSize>[] = [
	{ value: "13", label: "13px" },
	{ value: "14", label: "14px" },
];

export const AgentFontSizeSettings: FC = () => {
	const [size, setSize] = useAgentFontSize();
	return (
		<LocalPreferenceSelect
			title="Font size"
			description="Base text size for chats, the sidebar, and tool output. Smaller labels keep their size."
			value={size}
			options={fontSizeOptions}
			onChange={setSize}
		/>
	);
};
