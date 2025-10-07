import { css } from "@emotion/react";
import type { SerpentOption } from "api/typesGenerated";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
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
		<Table
			className="options-table"
			css={css`
          & td {
            padding-top: 24px;
            padding-bottom: 24px;
          }

          & td:last-child,
          & th:last-child {
            padding-left: 32px;
          }
        `}
		>
			<TableHead>
				<TableRow>
					<TableCell width="50%">Option</TableCell>
					<TableCell width="50%">Value</TableCell>
				</TableRow>
			</TableHead>
			<TableBody>
				{Object.values(options).map((option) => {
					return (
						<TableRow key={option.flag} className={`option-${option.flag}`}>
							<TableCell>
								<OptionName>{option.name}</OptionName>
								<OptionDescription>{option.description}</OptionDescription>
								<div
									css={{
										marginTop: 24,
										display: "flex",
										flexWrap: "wrap",
										gap: 8,
									}}
								>
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
