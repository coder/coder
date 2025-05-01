import { useTheme } from "@emotion/react";
import FormControl from "@mui/material/FormControl";
import InputLabel from "@mui/material/InputLabel";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import { deploymentLanguageModels } from "api/queries/deployment";
import type { LanguageModel } from "api/typesGenerated"; // Assuming types live here based on project structure
import { Loader } from "components/Loader/Loader";
import { ChangeEvent, type FC } from "react";
import { useQuery } from "react-query";
import { useChatContext } from "./ChatLayout";

export const LanguageModelSelector: FC = () => {
	const theme = useTheme();
	const { setSelectedModel, modelConfig, selectedModel } = useChatContext();
	const {
		data: languageModelConfig,
		isLoading,
		error,
	} = useQuery(deploymentLanguageModels());

	if (isLoading) {
		return <Loader size={14} />;
	}

	if (error || !languageModelConfig) {
		console.error("Failed to load language models:", error);
		return (
			<div css={{ color: theme.palette.error.main }}>Error loading models.</div>
		);
	}

	const models = languageModelConfig.models ?? [];

	if (models.length === 0) {
		return (
			<div css={{ color: theme.palette.text.disabled }}>
				No language models available.
			</div>
		);
	}

	return (
		<FormControl fullWidth size="small">
			<InputLabel id="model-select-label">Model</InputLabel>
			<Select
				labelId="model-select-label"
				value={selectedModel}
				label="Model"
				onChange={(e) => setSelectedModel(e.target.value)}
				disabled={isLoading || models.length === 0}
			>
				{!selectedModel && (
					<MenuItem value="" disabled>
						Select a model...
					</MenuItem>
				)}
				{models.map((model: LanguageModel) => (
					<MenuItem key={model.id} value={model.id}>
						{model.display_name} ({model.provider})
					</MenuItem>
				))}
			</Select>
		</FormControl>
	);
};
