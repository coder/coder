import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Link } from "#/components/Link/Link";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface VirtualDesktopSettingsProps {
	desktopEnabledData: TypesGen.ChatDesktopEnabledResponse | undefined;
	onSaveDesktopEnabled: (
		req: TypesGen.UpdateChatDesktopEnabledRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingDesktopEnabled: boolean;
	isSaveDesktopEnabledError: boolean;
	computerUseProviderData: TypesGen.ChatComputerUseProviderResponse | undefined;
	onSaveComputerUseProvider: (
		req: TypesGen.UpdateChatComputerUseProviderRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingComputerUseProvider: boolean;
	computerUseProviderSaveError: Error | null;
}

const defaultComputerUseProvider = "anthropic";

const computerUseProviderOptions = [
	{ label: "Anthropic", value: "anthropic" },
	{ label: "OpenAI", value: "openai" },
] as const;

const getComputerUseProviderLabel = (provider: string) => {
	return (
		computerUseProviderOptions.find((option) => option.value === provider)
			?.label ?? provider
	);
};

export const VirtualDesktopSettings: FC<VirtualDesktopSettingsProps> = ({
	desktopEnabledData,
	onSaveDesktopEnabled,
	isSavingDesktopEnabled,
	isSaveDesktopEnabledError,
	computerUseProviderData,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
}) => {
	const desktopEnabled = desktopEnabledData?.enable_desktop ?? false;
	const computerUseProvider =
		computerUseProviderData?.provider || defaultComputerUseProvider;

	return (
		<div className="flex flex-col gap-2">
			<div className="flex items-center justify-between gap-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Virtual Desktop
					</h3>
					<Badge size="sm" variant="warning" className="cursor-default">
						<TriangleAlertIcon className="h-3 w-3" />
						Experimental feature
					</Badge>
				</div>
				<Switch
					checked={desktopEnabled}
					onCheckedChange={(checked) =>
						onSaveDesktopEnabled({ enable_desktop: checked })
					}
					aria-label="Enable"
					disabled={isSavingDesktopEnabled}
				/>
			</div>
			<div className="m-0 flex-1 text-xs text-content-secondary">
				<p className="m-0">
					Allow agents to use a virtual, graphical desktop within workspaces.
					Requires the{" "}
					<Link
						href="https://registry.coder.com/modules/coder/portabledesktop"
						target="_blank"
						size="sm"
					>
						portabledesktop module
					</Link>{" "}
					to be installed in the workspace and the Anthropic provider to be
					configured.
				</p>
			</div>
			<div className="flex flex-col gap-2 pt-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
				<div className="flex flex-col gap-1">
					<h4
						id="computer-use-provider-label"
						className="m-0 text-sm font-semibold text-content-primary"
					>
						Computer use provider
					</h4>
					<p
						id="computer-use-provider-description"
						className="m-0 text-xs text-content-secondary"
					>
						Select the provider agents use for computer-use actions.
					</p>
				</div>
				<Select
					value={computerUseProvider}
					onValueChange={(provider) => onSaveComputerUseProvider({ provider })}
					disabled={isSavingComputerUseProvider}
				>
					<SelectTrigger
						aria-labelledby="computer-use-provider-label"
						aria-describedby="computer-use-provider-description"
						className="w-full sm:w-44"
					>
						<SelectValue>
							{getComputerUseProviderLabel(computerUseProvider)}
						</SelectValue>
					</SelectTrigger>
					<SelectContent align="end" className="min-w-[11rem]">
						<SelectGroup>
							{computerUseProviderOptions.map((option) => (
								<SelectItem key={option.value} value={option.value}>
									{option.label}
								</SelectItem>
							))}
						</SelectGroup>
					</SelectContent>
				</Select>
			</div>
			{isSaveDesktopEnabledError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save desktop setting.
				</p>
			)}
			{computerUseProviderSaveError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save computer use provider.
				</p>
			)}
		</div>
	);
};
