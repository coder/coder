import type { SerpentOption } from "#/api/typesGenerated";
import { PremiumBadge } from "#/components/Badge/PresetBadges";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	OptionConfig,
	OptionConfigFlag,
	OptionDescription,
	OptionName,
	OptionValue,
} from "./Option";
import { optionValue } from "./optionValue";

type OptionsTableProps = {
	options: readonly SerpentOption[];
	additionalValues?: readonly string[];
	/** When false, options annotated `enterprise` show a Premium badge. */
	hasLicense?: boolean;
};

const OptionsTable: React.FC<OptionsTableProps> = ({
	options,
	additionalValues,
	hasLicense = true,
}) => {
	if (options.length === 0) {
		return <p>No options to configure</p>;
	}

	return (
		<Table className="options-table">
			<TableHeader>
				<TableRow>
					<TableHead className="w-1/2">Option</TableHead>
					<TableHead className="w-1/2">Value</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{Object.values(options).map((option) => {
					return (
						<TableRow key={option.flag} className={`option-${option.flag}`}>
							<TableCell>
								<div className="flex items-center gap-2">
									<OptionName>{option.name}</OptionName>
									{!hasLicense && option.annotations?.enterprise === "true" && (
										<PremiumBadge />
									)}
								</div>
								<OptionDescription>{option.description}</OptionDescription>
								<div className="pt-2 flex flex-wrap gap-2">
									{option.flag && (
										<OptionConfig isSource={option.value_source === "flag"}>
											<OptionConfigFlag>CLI</OptionConfigFlag>
											--{option.flag}
										</OptionConfig>
									)}
									{option.flag_shorthand && (
										<OptionConfig isSource={option.value_source === "flag"}>
											<OptionConfigFlag>CLI</OptionConfigFlag>-
											{option.flag_shorthand}
										</OptionConfig>
									)}
									{option.env && (
										<OptionConfig isSource={option.value_source === "env"}>
											<OptionConfigFlag>ENV</OptionConfigFlag>
											{option.env}
										</OptionConfig>
									)}
									{option.yaml && (
										<OptionConfig isSource={option.value_source === "yaml"}>
											<OptionConfigFlag>YAML</OptionConfigFlag>
											{option.yaml}
										</OptionConfig>
									)}
								</div>
							</TableCell>

							<TableCell>
								<OptionValue>
									{optionValue(option, additionalValues)}
								</OptionValue>
							</TableCell>
						</TableRow>
					);
				})}
			</TableBody>
		</Table>
	);
};

export default OptionsTable;
