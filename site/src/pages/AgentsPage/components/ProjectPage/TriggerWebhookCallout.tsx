import { ChevronRightIcon, XIcon } from "lucide-react";
import { useId } from "react";
import type { ChatProjectTrigger } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { SettingsCardRow } from "./ProjectSection";
import type { TriggerChipProps } from "./TriggerSentenceChip";
import { TriggerSetupSteps } from "./TriggerSetupSteps";
import { triggerSources } from "./triggerCatalog";

type TriggerWebhookCalloutProps = {
	readonly trigger: ChatProjectTrigger;
	/** Embeds the secret. The server cannot show it again. */
	readonly webhookURL: string;
	/** Saves settings the setup steps ask for, such as a signing secret. */
	readonly onUpdate: TriggerChipProps["onUpdate"];
	readonly onDismiss: () => void;
};

/** A card row under a trigger that shows its new webhook URL once. */
export const TriggerWebhookCallout: React.FC<TriggerWebhookCalloutProps> = ({
	trigger,
	webhookURL,
	onUpdate,
	onDismiss,
}) => {
	const urlId = useId();
	return (
		<SettingsCardRow className="flex-col items-stretch gap-3 bg-surface-secondary">
			<div className="flex items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-1">
					<Label htmlFor={urlId}>Webhook URL</Label>
					<p className="m-0 text-xs text-content-secondary">
						Copy it now. It contains a secret and is shown only once.
					</p>
				</div>
				<Button
					size="icon"
					variant="subtle"
					aria-label="Dismiss webhook URL"
					onClick={onDismiss}
				>
					<XIcon />
				</Button>
			</div>
			<div className="flex items-center gap-2">
				<Input
					id={urlId}
					readOnly
					value={webhookURL}
					className="h-8 bg-surface-primary font-mono text-xs"
					onFocus={(event) => event.target.select()}
				/>
				<CopyButton text={webhookURL} label="Copy webhook URL" />
			</div>
			<Collapsible>
				<CollapsibleTrigger asChild>
					<Button variant="subtle" size="sm" className="group -ml-2 w-fit">
						<ChevronRightIcon className="transition-transform group-data-[state=open]:rotate-90" />
						{trigger.source === "webhook"
							? "How to call it"
							: `Set it up in ${triggerSources[trigger.source].title}`}
					</Button>
				</CollapsibleTrigger>
				<CollapsibleContent className="pt-2">
					<TriggerSetupSteps
						trigger={trigger}
						webhookURL={webhookURL}
						onUpdate={onUpdate}
					/>
				</CollapsibleContent>
			</Collapsible>
		</SettingsCardRow>
	);
};
