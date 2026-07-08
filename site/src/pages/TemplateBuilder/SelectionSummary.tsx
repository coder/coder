import { cva } from "class-variance-authority";
import { XIcon } from "lucide-react";
import { createContext, type PropsWithChildren, useContext } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";

type Variant = "complete" | "current" | "upcoming" | null | undefined;

const VariantContext = createContext<Variant>(null);

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
			<h2 className="text-xl font-semibold">Selection</h2>
			<div className="text-sm">
				<VariantContext.Provider value={variant(1)}>
					<StepIndicator step={1}>Base Template</StepIndicator>
					{selectedTemplate ? (
						<BaseTemplateSelection template={selectedTemplate} />
					) : (
						<StepDivider />
					)}
				</VariantContext.Provider>
				<VariantContext.Provider value={variant(2)}>
					<StepIndicator step={2}>Modules</StepIndicator>
					{selectedModules ? (
						<ModuleSelection
							modules={selectedModules}
							onDeselectModule={onDeselectModule}
						/>
					) : (
						<StepDivider />
					)}
				</VariantContext.Provider>
				<VariantContext.Provider value={variant(3)}>
					<StepIndicator step={3}>Customizations</StepIndicator>
				</VariantContext.Provider>
			</div>
		</div>
	);
};

const stepCircleVariants = cva(
	"rounded-full size-6 border border-solid flex items-center justify-center text-xs",
	{
		variants: {
			variant: {
				complete: "border-border-success bg-surface-green",
				current: "border-border-success",
				upcoming: "border-border text-content-disabled",
			},
		},
	},
);

const stepLabelVariants = cva("font-normal mr-2", {
	variants: {
		variant: {
			complete: "text-content-primary",
			current: "text-content-primary",
			upcoming: "text-content-disabled",
		},
	},
});

type StepIndicatorProps = PropsWithChildren<{
	step: number;
}>;

const StepIndicator: React.FC<StepIndicatorProps> = ({ step, children }) => {
	const variant = useContext(VariantContext);

	return (
		<div className="flex items-center gap-2">
			<div className={stepCircleVariants({ variant })}>{step}</div>
			<span className={stepLabelVariants({ variant })}>{children}</span>
		</div>
	);
};

const stepDividerVariants = cva(
	"border-0 border-l border-solid mx-3 -translate-x-px",
	{
		variants: {
			variant: {
				complete: "border-border-success",
				current: "border-border",
				upcoming: "border-border",
			},
		},
	},
);

type StepDividerProps = PropsWithChildren<{
	className?: string;
}>;

const StepDivider: React.FC<StepDividerProps> = ({ className, children }) => {
	const variant = useContext(VariantContext);

	return (
		<div
			className={cn(
				stepDividerVariants({ variant }),
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
			<div className="flex items-start p-1">
				<div className="h-[1lh] content-center">
					<Avatar src={template.iconUrl} size="sm" variant="icon" />
				</div>
				<span className="ml-2 text-content-secondary">{template.name}</span>
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
					className="group flex items-start justify-between p-1 mb-1 rounded-sm hover:bg-surface-secondary"
				>
					<div className="h-[1lh] content-center">
						<Avatar src={module.iconUrl} size="sm" variant="icon" />
					</div>
					<span className="flex-1 ml-2 text-content-secondary">
						{module.name}
					</span>
					<div className="h-[1lh] content-center">
						<Button
							size="xs"
							variant="subtle"
							className="flex opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
							onClick={() => onDeselectModule(module.id)}
							aria-label="Deselect module"
						>
							<XIcon className="size-4" />
						</Button>
					</div>
				</div>
			))}
		</StepDivider>
	);
};
