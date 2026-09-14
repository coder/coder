import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useState } from "react";
import type { ConcreteThemeName } from "#/theme";
import type { ThemeModeDraft } from "#/theme/themeMode";
import { Section } from "../Section";
import { SingleModeSection } from "./SingleModeSection";
import { SyncModeSection } from "./SyncModeSection";

interface ThemeModeSectionsStoryProps {
	activeScheme: "dark" | "light";
	mode: "single" | "sync";
}

const meta: Meta<ThemeModeSectionsStoryProps> = {
	title: "pages/UserSettingsPage/ThemeModeSections",
	args: {
		activeScheme: "light",
		mode: "sync",
	},
	argTypes: {
		activeScheme: {
			control: "radio",
			options: ["light", "dark"],
		},
		mode: {
			control: "radio",
			options: ["sync", "single"],
		},
	},
	render: (args) => <ThemeModeSectionsStory {...args} />,
};

export default meta;
type Story = StoryObj<ThemeModeSectionsStoryProps>;

const initialDraft: ThemeModeDraft = {
	mode: "sync",
	single: "dark",
	light: "light",
	dark: "dark",
};

const ThemeModeSectionsStory: FC<ThemeModeSectionsStoryProps> = ({
	activeScheme,
	mode,
}) => {
	const [draft, setDraft] = useState(initialDraft);
	const selectSingle = (theme: ConcreteThemeName) => {
		setDraft((current) => ({ ...current, single: theme }));
	};
	const selectSync = (scheme: "dark" | "light", theme: ConcreteThemeName) => {
		setDraft((current) => ({ ...current, [scheme]: theme }));
	};

	return (
		<div className="max-w-5xl p-8">
			<Section title="Theme" layout="fluid">
				{mode === "sync" ? (
					<SyncModeSection
						light={draft.light}
						dark={draft.dark}
						activeScheme={activeScheme}
						onSelect={selectSync}
					/>
				) : (
					<SingleModeSection selected={draft.single} onSelect={selectSingle} />
				)}
			</Section>
		</div>
	);
};

export const Sections: Story = {};
