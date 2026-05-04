import {
	type FormikHelpers,
	type FormikProps,
	type FormikValues,
	useFormik,
} from "formik";

interface UseModelOverrideFormOptions<TValues extends FormikValues> {
	initialValues: TValues;
	onSubmit: (values: TValues, helpers: FormikHelpers<TValues>) => void;
	isLoading: boolean;
	isSaving: boolean;
	disabled: boolean;
	hasLoadedOverride: boolean;
	isMalformedOverride: boolean;
}

interface UseModelOverrideFormResult<TValues extends FormikValues> {
	form: FormikProps<TValues>;
	isFormDisabled: boolean;
	canSave: boolean;
}

export const useModelOverrideForm = <TValues extends FormikValues>({
	initialValues,
	onSubmit,
	isLoading,
	isSaving,
	disabled,
	hasLoadedOverride,
	isMalformedOverride,
}: UseModelOverrideFormOptions<TValues>): UseModelOverrideFormResult<TValues> => {
	const form = useFormik<TValues>({
		enableReinitialize: true,
		initialValues,
		onSubmit,
	});
	const isFormDisabled =
		disabled || isSaving || isLoading || !hasLoadedOverride;
	const canSave =
		hasLoadedOverride && !disabled && (form.dirty || isMalformedOverride);

	return { form, isFormDisabled, canSave };
};
