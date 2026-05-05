import { XIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";

type SelectedTemplate = {
	name: string;
	iconUrl?: string;
};

type SelectedModule = {
	id: string;
	name: string;
	iconUrl: string;
};

type SelectionSummaryProps = {
	currentStep: number;
	selectedTemplate?: SelectedTemplate;
	selectedModules?: SelectedModule[];
	onDeselectModule: (moduleId: string) => void;
};

export const SelectionSummary: React.FC<SelectionSummaryProps> = ({
	currentStep,
	selectedTemplate,
	selectedModules,
	onDeselectModule,
}) => {
	const variant = (step: number) => {
		if (currentStep === step) return "current";
		if (currentStep > step) return "complete";
		return "upcoming";
	};
	return (
		<div>
			<h2 className="font-semibold">Selection</h2>
			<div className="flex flex-col">
				<StepIndicator step={1} variant={variant(1)}>
					Base Template
				</StepIndicator>
				{selectedTemplate ? (
					<BaseTemplateSelection template={selectedTemplate} />
				) : (
					<StepDivider />
				)}
				<StepIndicator step={2} variant={variant(2)}>
					Modules
				</StepIndicator>
				{selectedModules ? (
					<ModuleSelection
						modules={selectedModules}
						onDeselectModule={onDeselectModule}
					/>
				) : (
					<StepDivider />
				)}
				<StepIndicator step={3} variant={variant(3)}>
					Customizations
				</StepIndicator>
			</div>
		</div>
	);
};

const StepIndicator: React.FC<{
	step: number;
	variant: "complete" | "current" | "upcoming";
	children: React.ReactNode;
}> = ({ step, variant, children }) => {
	return (
		<div className="flex items-center justify-start gap-2">
			<div
				className={cn(
					"rounded-full w-8 h-8",
					"border border-border border-solid",
					"flex items-center justify-center",
					variant === "complete" && "border-border-success",
					variant === "current" && "border-content-primary",
					variant === "upcoming" && "border-border text-content-secondary",
				)}
			>
				{step}
			</div>
			<span
				className={cn(
					"font-normal mr-2",
					variant === "complete" && "text-content-primary",
					variant === "current" && "text-content-primary",
					variant === "upcoming" && "text-content-secondary",
				)}
			>
				{children}
			</span>
		</div>
	);
};

const StepDivider: React.FC<{
	children?: React.ReactNode;
	className?: string;
}> = ({ children, className }) => {
	return (
		<div
			className={cn(
				"border-0 border-l border-border border-solid mx-4 -translate-x-px",
				children ? "px-3 py-2" : "h-4",
				className,
			)}
		>
			{children}
		</div>
	);
};

const BaseTemplateSelection: React.FC<{ template: SelectedTemplate }> = ({
	template,
}) => {
	return (
		<StepDivider>
			<div className="flex items-center p-1">
				<img
					src={template.iconUrl}
					alt={`${template.name} icon`}
					className="w-6 h-6 p-1 rounded-sm border border-border border-solid bg-surface-secondary"
				/>
				<span className="ml-2">{template.name}</span>
			</div>
		</StepDivider>
	);
};

const ModuleSelection: React.FC<{
	modules: SelectedModule[];
	onDeselectModule: (moduleId: string) => void;
}> = ({ modules, onDeselectModule }) => {
	return (
		<StepDivider className="max-h-72 overflow-y-auto">
			{modules.map((module) => (
				<div
					key={module.id}
					className="group flex items-center justify-between p-1 mb-1 hover:bg-surface-secondary"
				>
					<div className="flex items-center">
						<img
							src={module.iconUrl}
							alt={`${module.name} icon`}
							className="w-6 h-6 p-1 rounded-sm border border-border border-solid bg-surface-secondary"
						/>
						<span className="ml-2">{module.name}</span>
					</div>
					<Button
						size="xs"
						variant="subtle"
						className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
						onClick={() => onDeselectModule(module.id)}
					>
						<XIcon className="w-4 h-4" />
					</Button>
				</div>
			))}
		</StepDivider>
	);
};
