import { Button, type ButtonProps } from "components/Button/Button";

export const AgentButton: React.FC<ButtonProps> = ({ ...props }) => {
	return <Button variant="outline" {...props} />;
};
