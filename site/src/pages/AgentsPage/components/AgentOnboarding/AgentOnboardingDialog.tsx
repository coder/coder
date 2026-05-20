import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { chatModelConfigs, chatProviderConfigs } from "#/api/queries/chats";
import { Dialog, DialogContent } from "#/components/Dialog/Dialog";
import { ExtendStep } from "./ExtendStep";
import { ModelStep } from "./ModelStep";
import { dismissOnboarding } from "./onboardingState";
import { ProviderStep } from "./ProviderStep";

type WizardStep = "provider" | "model" | "extend";

interface AgentOnboardingDialogProps {
	open: boolean;
	onClose: () => void;
}

export const AgentOnboardingDialog: FC<AgentOnboardingDialogProps> = ({
	open,
	onClose,
}) => {
	const [step, setStep] = useState<WizardStep>("provider");

	const providerConfigsQuery = useQuery(chatProviderConfigs());
	const modelConfigsQuery = useQuery(chatModelConfigs());

	const savedProviders = providerConfigsQuery.data ?? [];
	const savedModels = modelConfigsQuery.data ?? [];

	const handleSkip = () => {
		dismissOnboarding();
		onClose();
	};

	const handleFinish = () => {
		dismissOnboarding();
		onClose();
	};

	// Prevent closing via escape or clicking outside.
	const preventClose = (event: Event) => {
		event.preventDefault();
	};

	return (
		<Dialog open={open}>
			<DialogContent
				className="max-w-3xl gap-0 p-6 h-[540px]"
				onEscapeKeyDown={preventClose}
				onPointerDownOutside={preventClose}
			>
				{step === "provider" && (
					<ProviderStep
						onSkip={handleSkip}
						onContinue={() => setStep("model")}
					/>
				)}
				{step === "model" && (
					<ModelStep
						savedProviders={savedProviders}
						savedModels={savedModels}
						onBack={() => setStep("provider")}
						onSkip={handleSkip}
						onContinue={() => setStep("extend")}
					/>
				)}
				{step === "extend" && (
					<ExtendStep onBack={() => setStep("model")} onFinish={handleFinish} />
				)}
			</DialogContent>
		</Dialog>
	);
};
