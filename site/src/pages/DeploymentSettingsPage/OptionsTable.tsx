import type { SerpentOption } from "api/typesGenerated";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import {
	OptionConfig,
	OptionConfigFlag,
	OptionDescription,
	OptionName,
	OptionValue,
} from "./Option";
import { optionValue } from "./optionValue";

interface OptionsTableProps {
	options: readonly SerpentOption[];
	additionalValues?: readonly string[];
}

const OptionsTable: FC<OptionsTableProps> = ({ options, additionalValues }) => {
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
								<OptionName>{option.name}</OptionName>
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
