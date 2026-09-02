import {
	ArrowLeftIcon,
	CopyIcon,
	EllipsisVerticalIcon,
	ShieldIcon,
	TrashIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { getProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { cn } from "#/utils/cn";
import { useOrganizationModelsPath } from "../organizationModels";

export const ModelFormBackLink: FC = () => {
	const modelsPath = useOrganizationModelsPath();
	return (
		<Link to={modelsPath} className="-ml-3">
			<Button variant="subtle" type="button">
				<ArrowLeftIcon />
				<span>Back to models</span>
			</Button>
		</Link>
	);
};

export const ModelFormHeader: FC<{
	title: string;
	selectedProviderState: ProviderState;
	isEditing: boolean;
	editingModel?: TypesGen.ChatModel;
	onDeleteModel?: (modelId: string) => Promise<void>;
	onDuplicate?: () => void;
	onShareModel?: () => void;
	onToggleEnabled?: (enabled: boolean) => void;
	isSaving: boolean;
	enabledToggleDisabled: boolean;
	onRequestDelete: () => void;
}> = ({
	title,
	selectedProviderState,
	isEditing,
	editingModel,
	onDeleteModel,
	onDuplicate,
	onShareModel,
	onToggleEnabled,
	isSaving,
	enabledToggleDisabled,
	onRequestDelete,
}) => {
	return (
		<>
			<div className="flex items-center justify-between">
				<ModelFormBackLink />
				{isEditing &&
					editingModel &&
					(onDeleteModel || onDuplicate || onShareModel) && (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="subtle"
									size="icon"
									type="button"
									disabled={isSaving}
									aria-label="Model actions"
								>
									<EllipsisVerticalIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								{onShareModel && (
									<DropdownMenuItem onClick={onShareModel}>
										<ShieldIcon className="size-icon-sm" />
										Manage permissions
									</DropdownMenuItem>
								)}
								{onDuplicate && (
									<DropdownMenuItem onClick={onDuplicate}>
										<CopyIcon className="size-icon-sm" />
										Duplicate model
									</DropdownMenuItem>
								)}
								{onDeleteModel && (onShareModel || onDuplicate) && (
									<DropdownMenuSeparator />
								)}
								{onDeleteModel && (
									<DropdownMenuItem
										className="text-content-destructive focus:text-content-destructive"
										onClick={onRequestDelete}
									>
										<TrashIcon />
										Delete…
									</DropdownMenuItem>
								)}
							</DropdownMenuContent>
						</DropdownMenu>
					)}
			</div>
			<div className="flex items-center justify-between gap-4">
				<div className="flex items-center gap-4 min-w-0">
					<Avatar
						variant="icon"
						size="lg"
						src={getProviderIcon(selectedProviderState.provider)}
					/>
					<SettingsHeaderTitle>
						<span
							className={cn(
								"block min-w-0 truncate",
								editingModel?.enabled === false && "text-content-secondary",
							)}
						>
							{title}
						</span>
					</SettingsHeaderTitle>
					{isEditing && editingModel?.is_default && (
						<Badge variant="default">Default</Badge>
					)}
					{isEditing &&
						editingModel &&
						!editingModel.is_default &&
						!editingModel.enabled && <Badge variant="default">Disabled</Badge>}
				</div>
				{isEditing && editingModel && onToggleEnabled && (
					<div className="flex shrink-0 items-center gap-2">
						<Tooltip>
							<TooltipTrigger asChild>
								<span className="inline-flex">
									<Switch
										checked={editingModel.enabled}
										onCheckedChange={onToggleEnabled}
										disabled={enabledToggleDisabled}
										aria-label="Model enabled"
									/>
								</span>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								{editingModel.is_default && editingModel.enabled
									? "Default model cannot be disabled. Set another model as default first."
									: editingModel.enabled
										? "Disable this model. It will be hidden from users."
										: "Enable this model. It will be visible to users."}
							</TooltipContent>
						</Tooltip>
						<span className="text-sm">Enable</span>
					</div>
				)}
			</div>
		</>
	);
};
