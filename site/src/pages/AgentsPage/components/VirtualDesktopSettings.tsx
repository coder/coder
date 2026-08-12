import { useFormik } from "formik";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { AgentSettingLayout } from "#/pages/AISettingsPage/CoderAgentsPage/components/AgentSettingLayout";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface VirtualDesktopSettingsProps {
	computerUseProviderData: TypesGen.ChatComputerUseProviderResponse | undefined;
	isLoadingComputerUseProvider: boolean;
	onSaveComputerUseProvider: (
		req: TypesGen.UpdateChatComputerUseProviderRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingComputerUseProvider: boolean;
	computerUseProviderSaveError: Error | null;
}

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
	computerUseProviderData,
	isLoadingComputerUseProvider,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const serverProvider = computerUseProviderData?.provider ?? "";
	const hasLoaded = computerUseProviderData !== undefined;

	const form = useFormik<{ provider: TypesGen.ChatComputerUseProvider | "" }>({
		enableReinitialize: true,
		initialValues: {
			provider: serverProvider,
		},
		onSubmit: (values, helpers) => {
			if (!values.provider) {
				return;
			}
			onSaveComputerUseProvider(
				{ provider: values.provider },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const isFormDisabled =
		isSavingComputerUseProvider || isLoadingComputerUseProvider || !hasLoaded;
	const canSave = hasLoaded && form.dirty;

	return (
		<AgentSettingLayout
			title="Virtual desktop"
			description={
				<>
					Allow agents to use a virtual, graphical desktop within workspaces.
					Requires the{" "}
					<Link
						href="https://registry.coder.com/modules/coder/portabledesktop"
						target="_blank"
						size="sm"
					>
						portabledesktop module
					</Link>{" "}
					to be installed in the workspace and a computer use provider to be
					configured.
				</>
			}
			showSave={canSave}
			isSaving={isSavingComputerUseProvider}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				computerUseProviderSaveError ? (
					<p className="m-0">Failed to save computer use provider.</p>
				) : undefined
			}
		>
			<div className="flex w-[22rem] max-w-full flex-col gap-2">
				<Select
					value={form.values.provider}
					onValueChange={(value) => void form.setFieldValue("provider", value)}
					disabled={isFormDisabled}
				>
					<SelectTrigger
						aria-label="Computer use provider"
						className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-none"
					>
						<SelectValue placeholder="Select provider">
							{isLoadingComputerUseProvider ? (
								<Skeleton className="h-4 w-20" aria-hidden="true" />
							) : form.values.provider ? (
								getComputerUseProviderLabel(form.values.provider)
							) : undefined}
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
		</AgentSettingLayout>
	);
};
