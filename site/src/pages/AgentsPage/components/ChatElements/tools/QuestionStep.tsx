import { cn } from "cn";
import { CheckIcon, MessageCircleQuestionIcon } from "lucide-react";
import { RadioGroup as RadioGroupPrimitive } from "radix-ui";
import { Input } from "#/components/Input/Input";
import { Kbd } from "#/components/Kbd/Kbd";
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import { Response } from "../Response";
import type { AskUserQuestion } from "./AskUserQuestionTool";
import {
	getOptionLabel,
	getQuestionHeader,
	getQuestionLabel,
	getQuestionText,
	OTHER_OPTION_VALUE,
} from "./questionAnswers";
import type { QuestionDraft } from "./questionDraft";

const OTHER_PLACEHOLDER = "Something else: type your own answer";

const getSelectedValue = (draft: QuestionDraft): string => {
	if (typeof draft.selection === "number") {
		return `option-${draft.selection}`;
	}
	// An empty value keeps the radio group controlled with nothing selected.
	return draft.selection === "other" ? OTHER_OPTION_VALUE : "";
};

type QuestionOptionProps = {
	id: string;
	value: string;
	label: string;
	description?: string;
	shortcut: number;
	isSelected: boolean;
	isEnabled: boolean;
	ref: React.Ref<HTMLButtonElement>;
};

type OptionRowProps = {
	className?: string;
	isSelected: boolean;
	isEnabled: boolean;
	children: React.ReactNode;
};

/** The radios hide their focus outline and the Other field drops its ring, so the row shows focus. */
const OptionRow: React.FC<OptionRowProps> = ({
	className,
	isSelected,
	isEnabled,
	children,
}) => (
	<div
		className={cn(
			"rounded-lg border border-solid transition-colors",
			"has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-content-link",
			isSelected
				? "border-surface-invert-primary bg-surface-secondary"
				: cn("border-border", isEnabled && "hover:bg-surface-secondary/50"),
			className,
		)}
	>
		{children}
	</div>
);

type ShortcutBadgeProps = {
	shortcut: number;
	isSelected: boolean;
};

const ShortcutBadge: React.FC<ShortcutBadgeProps> = ({
	shortcut,
	isSelected,
}) => (
	<Kbd
		aria-hidden="true"
		className={cn(
			"shrink-0",
			isSelected && "bg-surface-invert-primary text-content-invert",
		)}
	>
		{shortcut}
	</Kbd>
);

const QuestionOption: React.FC<QuestionOptionProps> = ({
	id,
	value,
	label,
	description,
	shortcut,
	isSelected,
	isEnabled,
	ref,
}) => {
	const descriptionId = description ? `${id}-description` : undefined;
	return (
		<OptionRow isSelected={isSelected} isEnabled={isEnabled}>
			{/* The whole first line is the radio, so a click anywhere on it selects. */}
			<RadioGroupPrimitive.Item
				ref={ref}
				id={id}
				value={value}
				disabled={!isEnabled}
				aria-describedby={descriptionId}
				aria-keyshortcuts={String(shortcut)}
				className={cn(
					"flex w-full items-start gap-3 border-0 bg-transparent p-3 text-left font-sans text-sm font-normal text-content-primary",
					"focus-visible:outline-hidden disabled:cursor-default",
					description && "pb-0.5",
					isEnabled && "cursor-pointer",
				)}
			>
				<ShortcutBadge shortcut={shortcut} isSelected={isSelected} />
				<span className="min-w-0 flex-1 wrap-anywhere">{label}</span>
				{isSelected && (
					<CheckIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
				)}
			</RadioGroupPrimitive.Item>
			{/* Outside the radio: Markdown links and copy buttons must not nest in a button. */}
			{description && (
				<Response
					id={descriptionId}
					className="pr-3 pb-3 pl-11 wrap-anywhere text-content-secondary"
				>
					{description}
				</Response>
			)}
		</OptionRow>
	);
};

type OtherQuestionOptionProps = {
	id: string;
	questionHeader: string;
	otherText: string;
	shortcut: number;
	isSelected: boolean;
	isEnabled: boolean;
	textRef: React.Ref<HTMLInputElement>;
	onSelect: () => void;
	onTextChange: (text: string) => void;
};

/**
 * Clicking the field or typing in it selects "Other". Tabbing through it
 * does not, so a keyboard user's choice survives the way to Submit.
 */
const OtherQuestionOption: React.FC<OtherQuestionOptionProps> = ({
	id,
	questionHeader,
	otherText,
	shortcut,
	isSelected,
	isEnabled,
	textRef,
	onSelect,
	onTextChange,
}) => {
	return (
		<OptionRow
			className="flex items-center gap-3 py-1.5 pr-1.5 pl-3"
			isSelected={isSelected}
			isEnabled={isEnabled}
		>
			<RadioGroupPrimitive.Item
				id={id}
				value={OTHER_OPTION_VALUE}
				disabled={!isEnabled}
				aria-label="Other"
				aria-keyshortcuts={String(shortcut)}
				className={cn(
					"shrink-0 rounded-sm border-0 bg-transparent p-0 focus-visible:outline-hidden",
					isEnabled && "cursor-pointer",
				)}
			>
				<ShortcutBadge shortcut={shortcut} isSelected={isSelected} />
			</RadioGroupPrimitive.Item>
			<Input
				ref={textRef}
				aria-label={`${OTHER_PLACEHOLDER} for ${questionHeader}`}
				className="h-8 border-0 px-0 text-sm shadow-none focus-visible:ring-0"
				disabled={!isEnabled}
				placeholder={OTHER_PLACEHOLDER}
				value={otherText}
				onClick={onSelect}
				onChange={(event) => {
					onTextChange(event.currentTarget.value);
				}}
			/>
		</OptionRow>
	);
};

type QuestionStepProps = {
	question: AskUserQuestion;
	questionIndex: number;
	questionCount: number;
	idPrefix: string;
	draft: QuestionDraft;
	isEnabled: boolean;
	questionTextRef: React.Ref<HTMLDivElement>;
	optionRefs: React.RefObject<Array<HTMLButtonElement | null>>;
	otherTextRef: React.Ref<HTMLInputElement>;
	onOptionChange: (value: string) => void;
	onOtherSelect: () => void;
	onOtherTextChange: (text: string) => void;
};

/** One question with its answer options and the Other field. */
export const QuestionStep: React.FC<QuestionStepProps> = ({
	question,
	questionIndex,
	questionCount,
	idPrefix,
	draft,
	isEnabled,
	questionTextRef,
	optionRefs,
	otherTextRef,
	onOptionChange,
	onOtherSelect,
	onOtherTextChange,
}) => {
	const questionIdBase = `${idPrefix}-question-${questionIndex}`;
	const questionTextId = `${questionIdBase}-text`;
	const questionLabelId = `${questionIdBase}-label`;

	return (
		<div>
			<p
				id={questionLabelId}
				className="m-0 flex items-center gap-1.5 text-xs text-content-secondary"
			>
				<MessageCircleQuestionIcon aria-hidden="true" className="size-3.5" />
				{getQuestionLabel(question, questionIndex, questionCount)}
			</p>
			<Response
				ref={questionTextRef}
				id={questionTextId}
				tabIndex={-1}
				className="mt-1 text-[15px] font-medium focus:outline-hidden"
			>
				{getQuestionText(question)}
			</Response>
			<RadioGroup
				aria-labelledby={`${questionLabelId} ${questionTextId}`}
				className="mt-4 gap-1.5"
				name={`${questionIdBase}-options`}
				value={getSelectedValue(draft)}
				onValueChange={onOptionChange}
			>
				{question.options.map((option, optionIndex) => (
					<QuestionOption
						key={`${option.label}-${optionIndex}`}
						ref={(element) => {
							optionRefs.current[optionIndex] = element;
						}}
						id={`${questionIdBase}-option-${optionIndex}`}
						value={`option-${optionIndex}`}
						label={getOptionLabel(question, optionIndex)}
						description={option.description}
						shortcut={optionIndex + 1}
						isSelected={draft.selection === optionIndex}
						isEnabled={isEnabled}
					/>
				))}
				<OtherQuestionOption
					id={`${questionIdBase}-option-other`}
					questionHeader={getQuestionHeader(question, questionIndex)}
					otherText={draft.otherText}
					shortcut={question.options.length + 1}
					isSelected={draft.selection === "other"}
					isEnabled={isEnabled}
					textRef={otherTextRef}
					onSelect={onOtherSelect}
					onTextChange={onOtherTextChange}
				/>
			</RadioGroup>
		</div>
	);
};
