import { API } from "api/api";
import type { CreateTokenRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { CodeExample } from "components/CodeExample/CodeExample";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { useFormik } from "formik";
import { type FC, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { useNavigate } from "react-router";
import { pageTitle } from "utils/page";
import { CreateTokenForm } from "./CreateTokenForm";
import { useAllowListResolver } from "./useAllowListResolver";
import {
	buildRequestScopes,
	type CreateTokenData,
	NANO_HOUR,
	serializeAllowList,
} from "./utils";

const initialValues: CreateTokenData = {
	name: "",
	lifetime: 30,
	scopeMode: "composite",
	compositeScopes: [],
	lowLevelScopes: [],
	allowList: [],
};

const CreateTokenPage: FC = () => {
	const navigate = useNavigate();

	const {
		mutate: saveToken,
		isPending: isCreating,
		isError: creationFailed,
		isSuccess: creationSuccessful,
		data: newToken,
	} = useMutation({ mutationFn: API.createToken });
	const {
		data: tokenConfig,
		isLoading: fetchingTokenConfig,
		isError: tokenFetchFailed,
		error: tokenFetchError,
	} = useQuery({
		queryKey: ["tokenconfig"],
		queryFn: API.getTokenConfig,
	});
	const {
		data: scopeCatalog,
		isLoading: fetchingScopeCatalog,
		isError: scopeCatalogFailed,
		error: scopeCatalogError,
	} = useQuery({
		queryKey: ["scopecatalog"],
		queryFn: API.getScopeCatalog,
	});

	const [formError, setFormError] = useState<unknown>(undefined);

	const resolveAllowListOptions = useAllowListResolver();

	const onCreateSuccess = () => {
		displaySuccess("Token has been created");
		navigate("/settings/tokens");
	};

	const onCreateError = (error: unknown) => {
		setFormError(error);
		displayError("Failed to create token");
	};

	const form = useFormik<CreateTokenData>({
		initialValues,
		onSubmit: (values) => {
			const scopes = buildRequestScopes(
				values.compositeScopes,
				values.lowLevelScopes,
			);
			const allowList = serializeAllowList(values.allowList);

			const payload: CreateTokenRequest = {
				lifetime: values.lifetime * 24 * NANO_HOUR,
				token_name: values.name,
				...(scopes.length > 0 ? { scopes } : {}),
				...(allowList && allowList.length > 0 ? { allow_list: allowList } : {}),
			};

			saveToken(payload, {
				onError: onCreateError,
			});
		},
	});

	if (fetchingTokenConfig || fetchingScopeCatalog) {
		return <Loader />;
	}

	const tokenDescription = (
		<>
			<p>Make sure you copy the below token before proceeding:</p>
			<CodeExample
				secret={false}
				code={newToken?.key ?? ""}
				css={{
					minHeight: "auto",
					userSelect: "all",
					width: "100%",
					marginTop: 24,
				}}
			/>
		</>
	);

	return (
		<>
			<title>{pageTitle("Create Token")}</title>

			{tokenFetchFailed && <ErrorAlert error={tokenFetchError} />}
			{scopeCatalogFailed && <ErrorAlert error={scopeCatalogError} />}
			<FullPageHorizontalForm
				title="Create Token"
				detail="Choose the minimum set of scopes and optional resource allow-list before generating your token."
			>
				<CreateTokenForm
					form={form}
					maxTokenLifetime={tokenConfig?.max_token_lifetime}
					formError={formError}
					setFormError={setFormError}
					isSubmitting={isCreating}
					submitFailed={creationFailed}
					scopeCatalog={scopeCatalog}
					resolveAllowListOptions={resolveAllowListOptions}
				/>

				<ConfirmDialog
					type="info"
					hideCancel
					title="Creation successful"
					description={tokenDescription}
					open={creationSuccessful && Boolean(newToken.key)}
					confirmLoading={isCreating}
					onConfirm={onCreateSuccess}
					onClose={onCreateSuccess}
				/>
			</FullPageHorizontalForm>
		</>
	);
};

export default CreateTokenPage;
