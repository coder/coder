import { API } from "api/api";
import type {
	APIAllowListTarget,
	APIKey,
	ScopeCatalog,
	UpdateTokenRequest,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { useFormik } from "formik";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { pageTitle } from "utils/page";
import { CreateTokenForm } from "../CreateTokenPage/CreateTokenForm";
import { useAllowListResolver } from "../CreateTokenPage/useAllowListResolver";
import {
	allowListTargetsToStrings,
	buildRequestScopes,
	type CreateTokenData,
	NANO_HOUR,
	serializeAllowList,
} from "../CreateTokenPage/utils";

const defaultValues: CreateTokenData = {
	name: "",
	lifetime: 30,
	scopeMode: "composite",
	compositeScopes: [],
	lowLevelScopes: [],
	allowList: [],
};

const lifetimeDaysFromSeconds = (seconds: number) => {
	return Math.max(1, Math.ceil(seconds / (24 * 60 * 60)));
};

const scopesAreEqual = (a: readonly string[], b: readonly string[]) => {
	if (a.length !== b.length) {
		return false;
	}
	const setA = new Set(a);
	return b.every((scope) => setA.has(scope));
};

const allowListsEqual = (
	a?: readonly APIAllowListTarget[],
	b?: readonly APIAllowListTarget[],
): boolean => {
	const stringify = (targets?: readonly APIAllowListTarget[]) =>
		(targets ?? [])
			.map((target) => `${target.type}:${target.id}`)
			.sort()
			.join("|");
	return stringify(a) === stringify(b);
};

const convertTokenToFormValues = (
	token: APIKey,
	scopeCatalog?: ScopeCatalog,
): CreateTokenData => {
	const compositeNames = new Set(
		scopeCatalog?.composites?.map((item) => item.name) ?? [],
	);
	const compositeScopes = token.scopes.filter((scope) =>
		compositeNames.has(scope),
	);
	const lowLevelScopes = token.scopes.filter(
		(scope) => !compositeNames.has(scope),
	);

	return {
		name: token.token_name,
		lifetime: lifetimeDaysFromSeconds(token.lifetime_seconds),
		scopeMode: compositeScopes.length > 0 ? "composite" : "low_level",
		compositeScopes,
		lowLevelScopes,
		allowList: allowListTargetsToStrings(token.allow_list),
	};
};

const EditTokenPage = () => {
	const { tokenName } = useParams<{ tokenName?: string }>();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [formError, setFormError] = useState<unknown>(undefined);
	const resolveAllowListOptions = useAllowListResolver();

	const {
		data: token,
		isLoading: loadingToken,
		isError: tokenFailed,
		error: tokenError,
	} = useQuery({
		queryKey: ["token", tokenName],
		queryFn: () => API.getToken(tokenName ?? ""),
		enabled: Boolean(tokenName),
	});

	const {
		data: scopeCatalog,
		isLoading: loadingCatalog,
		isError: catalogFailed,
		error: catalogError,
	} = useQuery({
		queryKey: ["scopecatalog"],
		queryFn: API.getScopeCatalog,
	});

	const {
		data: tokenConfig,
		isLoading: loadingConfig,
		isError: configFailed,
		error: configError,
	} = useQuery({
		queryKey: ["tokenconfig"],
		queryFn: API.getTokenConfig,
	});

	const {
		mutate: runUpdate,
		isPending: isUpdating,
		isError: updateFailed,
	} = useMutation({
		mutationFn: (payload: UpdateTokenRequest) =>
			API.updateToken(tokenName ?? "", payload),
		onSuccess: async () => {
			displaySuccess("Token has been updated");
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ["token", tokenName] }),
				queryClient.invalidateQueries({ queryKey: ["tokens"] }),
			]);
			navigate("/settings/tokens");
		},
		onError: (error: unknown) => {
			setFormError(error);
			displayError("Failed to update token");
		},
	});

	const initialValues = token
		? convertTokenToFormValues(token, scopeCatalog)
		: defaultValues;

	const form = useFormik<CreateTokenData>({
		enableReinitialize: true,
		initialValues,
		onSubmit: (values) => {
			if (!token) {
				return;
			}

			const scopes = buildRequestScopes(
				values.compositeScopes,
				values.lowLevelScopes,
			);
			const allowList = serializeAllowList(values.allowList);
			const currentAllowList = token.allow_list;

			const scopesChanged = !scopesAreEqual(scopes, token.scopes);
			const newLifetime = values.lifetime * 24 * NANO_HOUR;
			const currentLifetime = token.lifetime_seconds * 1_000_000_000;
			const lifetimeChanged = newLifetime !== currentLifetime;
			const allowListChanged = !allowListsEqual(allowList, currentAllowList);
			const nextAllowList =
				values.allowList.length === 0 ? [] : (allowList ?? []);

			const payload: UpdateTokenRequest = {
				...(scopesChanged ? { scopes } : {}),
				...(lifetimeChanged ? { lifetime: newLifetime } : {}),
				...(allowListChanged ? { allow_list: nextAllowList } : {}),
			};

			if (Object.keys(payload).length === 0) {
				displaySuccess("Nothing to update");
				navigate("/settings/tokens");
				return;
			}

			runUpdate(payload);
		},
	});

	if (loadingToken || loadingCatalog || loadingConfig) {
		return <Loader />;
	}

	if (!tokenName || tokenFailed || !token) {
		return (
			<ErrorAlert
				error={tokenError ?? new Error("Token could not be loaded")}
			/>
		);
	}

	return (
		<>
			<title>{pageTitle(`Edit Token · ${token.token_name}`)}</title>
			{catalogFailed && <ErrorAlert error={catalogError} />}
			{configFailed && <ErrorAlert error={configError} />}
			{updateFailed && <ErrorAlert error={formError} />}
			<FullPageHorizontalForm
				title={`Edit ${token.token_name}`}
				detail="Adjust token permissions or allowed resources."
			>
				<CreateTokenForm
					form={form}
					maxTokenLifetime={tokenConfig?.max_token_lifetime}
					formError={formError}
					setFormError={setFormError}
					isSubmitting={isUpdating}
					submitFailed={updateFailed}
					scopeCatalog={scopeCatalog}
					initialAllowListTargets={token.allow_list}
					resolveAllowListOptions={resolveAllowListOptions}
					submitLabel="Save changes"
					nameDisabled
				/>
			</FullPageHorizontalForm>
		</>
	);
};

export default EditTokenPage;
