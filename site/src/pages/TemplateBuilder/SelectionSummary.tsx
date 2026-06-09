import { cva, type VariantProps } from "class-variance-authority";
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
			<div>
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

const stepCircleVariants = cva(
	"rounded-full w-8 h-8 border border-solid flex items-center justify-center",
	{
		variants: {
			variant: {
				complete: "border-border-success",
				current: "border-content-primary",
				upcoming: "border-border text-content-secondary",
			},
		},
	},
);

const stepLabelVariants = cva("font-normal mr-2", {
	variants: {
		variant: {
			complete: "text-content-primary",
			current: "text-content-primary",
			upcoming: "text-content-secondary",
		},
	},
});

type StepIndicatorProps = VariantProps<typeof stepCircleVariants> & {
	step: number;
	children: React.ReactNode;
};

const StepIndicator: React.FC<StepIndicatorProps> = ({
	step,
	variant,
	children,
}) => {
	return (
		<div className="flex items-center gap-2">
			<div className={stepCircleVariants({ variant })}>{step}</div>
			<span className={stepLabelVariants({ variant })}>{children}</span>
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

type BaseTemplateSelectionProps = {
	template: SelectedTemplate;
};

const BaseTemplateSelection: React.FC<BaseTemplateSelectionProps> = ({
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

type ModuleSelectionProps = {
	modules: SelectedModule[];
	onDeselectModule: (moduleId: string) => void;
};

const ModuleSelection: React.FC<ModuleSelectionProps> = ({
	modules,
	onDeselectModule,
}) => {
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
