import { type FC, useId } from "react";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";
import {
	useVimNavigationModifier,
	useVimNavigationSetting,
} from "../hooks/useVimNavigation";
import {
	getDefaultVimModifier,
	getModifierLabel,
	isVimModifier,
	VIM_MODIFIERS,
} from "../utils/keyboardShortcuts";

export const ChatVimNavigationSettings: FC = () => {
	const [enabled, setEnabled] = useVimNavigationSetting();
	const [modifier, setModifier] = useVimNavigationModifier();
	const descriptionId = useId();
	const modifierDescriptionId = useId();
	const mod = getModifierLabel(modifier);
	const platformMod = getDefaultVimModifier();
	const searchSentence =
		modifier === platformMod
			? `Search moves from ${mod}+K to ${mod}+/.`
			: `Search is also available on ${mod}+/.`;

	return (
		<div className="flex flex-col gap-4">
			<div className="flex items-center justify-between gap-4">
				<p
					id={descriptionId}
					className="m-0 flex-1 text-xs text-content-secondary"
				>
					Vim-style navigation. {mod}+J and {mod}+K select the next and previous
					chat, {mod}+Shift+J and {mod}+Shift+K jump to the last and first chat,{" "}
					{mod}+Shift+O starts a new chat, {mod}+Shift+E renames the current
					chat, and Escape on a sidebar chat returns focus to the message input.{" "}
					{searchSentence} These override browser shortcuts on the same keys.
				</p>
				<Switch
					checked={enabled}
					onCheckedChange={(checked) => setEnabled(Boolean(checked))}
					aria-label="Vim-style chat navigation"
					aria-describedby={descriptionId}
				/>
			</div>
			<div className="flex items-center justify-between gap-4">
				<p
					id={modifierDescriptionId}
					className="m-0 flex-1 text-xs text-content-secondary"
				>
					Modifier key held for the vim-style navigation shortcuts.
				</p>
				<Select
					value={modifier}
					onValueChange={(value: string) => {
						if (isVimModifier(value)) {
							setModifier(value);
						}
					}}
				>
					<SelectTrigger
						className="w-44 shrink-0"
						aria-label="Vim navigation modifier"
						aria-describedby={modifierDescriptionId}
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{VIM_MODIFIERS.map((value) => (
							<SelectItem key={value} value={value}>
								{getModifierLabel(value)}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
		</div>
	);
};
